package main

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
)

var (
	globalModelJSON []byte // Models cache, store as JSON bytes
	modelDataMutex  sync.RWMutex
)

// parseProvider splits the URL path to determine the provider and remaining path
func parseProvider(r *http.Request) (provider, remainingPath string) {
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// proxyHeaders modifies headers for the request based on provider settings
func proxyHeaders(original http.Header, provider ProviderConfig) http.Header {
	headers := make(http.Header)
	for k, v := range original {
		if k != "Host" && k != "Cookie" && k != "Origin" {
			headers[k] = v
		}
	}

	if provider.ApiKey != "" {
		headers.Del("Authorization")
		headers.Set("Authorization", "Bearer "+provider.ApiKey)
	}

	for orig, mapped := range provider.HeaderMapping {
		if val := headers.Get(orig); val != "" {
			headers.Del(orig)
			headers.Set(mapped, val)
		}
	}

	return headers
}

// decompressBody handles gzip and deflate encoded response bodies
func decompressBody(body io.Reader, encoding string) (io.Reader, error) {
	switch strings.ToLower(encoding) {
	case "gzip":
		return gzip.NewReader(body)
	case "deflate":
		return zlib.NewReader(body)
	case "br":
		return brotli.NewReader(body), nil
	case "zstd":
		return zstd.NewReader(body)
	case "snappy":
		return snappy.NewReader(body), nil
	default:
		return body, nil
	}
}

// processChunks processes the streaming response body chunks and reconstructs the complete object
func processChunks(chunks []byte, totalChunks int) string {
	type Metadata struct {
		ID                string `json:"id"`
		Model             string `json:"model"`
		SystemFingerprint string `json:"system_fingerprint"`
		Created           int64  `json:"created"`
	}

	type Response struct {
		Metadata    Metadata         `json:"metadata"`
		Content     string           `json:"content"`
		Usage       *json.RawMessage `json:"usage"`
		TotalChunks int              `json:"total_chunks"`
	}

	var assembledContent strings.Builder
	var usageStats json.RawMessage
	var isOpenAISpec = false
	metadata := Metadata{}

	lines := strings.Split(string(chunks), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			isOpenAISpec = true
			data := strings.TrimPrefix(line, "data: ")
			data = strings.TrimSpace(data)
			if data == "[DONE]" {
				break
			}

			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(data), &parsed); err != nil {
				continue
			}

			// Store metadata from first chunk
			if metadata.ID == "" {
				if id, ok := parsed["id"].(string); ok {
					metadata.ID = id
				}
				if model, ok := parsed["model"].(string); ok {
					metadata.Model = model
				}
				if fingerprint, ok := parsed["system_fingerprint"].(string); ok {
					metadata.SystemFingerprint = fingerprint
				}
				if created, ok := parsed["created"].(float64); ok {
					metadata.Created = int64(created)
				}
			}

			// Handle content chunks
			if choices, ok := parsed["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						if content, ok := delta["content"].(string); ok {
							assembledContent.WriteString(content)
						}
					}
				}
			}

			// Capture usage statistics from final chunk
			if usage, ok := parsed["usage"]; ok {
				usageStats, _ = json.Marshal(usage)
			}
		}
	}

	if isOpenAISpec {
		response := Response{
			Metadata:    metadata,
			Content:     strings.TrimSpace(assembledContent.String()),
			Usage:       &usageStats,
			TotalChunks: totalChunks,
		}
		jsonResp, _ := json.MarshalIndent(response, "", "  ")
		return string(jsonResp)

	} else {
		return string(chunks)
	}
}

func (s *ProxyServer) handleCORS(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return true
	}

	return false
}

func (s *ProxyServer) fetchRaw(ctx context.Context, headers http.Header, method string, url string) ([]byte, http.Header, error) {
	r, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create context: %w", err)
	}

	r.Header = headers

	resp, err := s.client.Do(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	respBuffer, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return respBuffer, resp.Header, nil
}

func (s *ProxyServer) fetchJSON(ctx context.Context, headers http.Header, method string, url string) (map[string]interface{}, error) {
	respBuffer, respHeader, err := s.fetchRaw(ctx, headers, method, url)
	if err != nil {
		return nil, err
	}

	encoding := respHeader.Get("Content-Encoding")
	if encoding != "" {
		reader, err := decompressBody(bytes.NewReader(respBuffer), encoding)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress response: %w", err)
		}
		decompressedBody, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to read decompressed: %w", err)
		}
		respBuffer = decompressedBody
	}

	var jsonResp map[string]interface{}
	if err := json.Unmarshal(respBuffer, &jsonResp); err != nil {
		return nil, fmt.Errorf("failed to convert to JSON: %w", err)
	}

	return jsonResp, nil
}

func (s *ProxyServer) getGeminiModels(ctx context.Context, provider string, providerConfig ProviderConfig) ([]map[string]interface{}, error) {
	url := geminiBaseURL + "/models?key=" + s.config.Providers[provider].ApiKey
	headers := proxyHeaders(http.Header{}, providerConfig)
	headers.Del("Authorization")
	jsonResp, err := s.fetchJSON(ctx, headers, "GET", url)
	if err != nil {
		return nil, err
	}

	var providerModelData []map[string]interface{}
	if data, ok := jsonResp["models"].([]interface{}); ok {
		for _, item := range data {
			if obj, ok := item.(map[string]interface{}); ok {
				if name, exists := obj["name"].(string); exists {
					name = strings.Replace(name, "models/", "", 1)
					obj["name"] = provider + ":" + name
					obj["id"] = obj["name"]
				}
				providerModelData = append(providerModelData, obj)
			}
		}
	}
	return providerModelData, nil
}

func (s *ProxyServer) getModels(ctx context.Context, provider string, providerConfig ProviderConfig) ([]map[string]interface{}, error) {
	if providerConfig.IsGemini {
		return s.getGeminiModels(ctx, provider, providerConfig)
	}

	url := providerConfig.BaseURL + "/models"
	headers := proxyHeaders(http.Header{}, providerConfig)
	parsed, err := s.fetchJSON(ctx, headers, "GET", url)

	if err != nil {
		return nil, err
	}

	var providerModelData []map[string]interface{}
	if data, ok := parsed["data"].([]interface{}); ok {
		for _, item := range data {
			if obj, ok := item.(map[string]interface{}); ok {
				if id, exists := obj["id"].(string); exists {
					obj["id"] = provider + ":" + id
				}
				if name, exists := obj["name"].(string); exists {
					obj["name"] = provider + ":" + name
				}
				providerModelData = append(providerModelData, obj)
			}
		}
	}
	return providerModelData, nil
}

func (s *ProxyServer) getAllModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	modelDataMutex.RLock()
	if globalModelJSON != nil {
		_, err := w.Write(globalModelJSON)
		if err != nil {
			log.Printf("failed writing model cache: %v", err)
		}
		modelDataMutex.RUnlock()
		return
	}
	modelDataMutex.RUnlock()

	ctx, cancel := context.WithTimeout(r.Context(), httpRequestTimeout)
	defer cancel()

	var combinedModelData []map[string]interface{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	for provider, providerConfig := range s.config.Providers {
		wg.Add(1)
		go func(p string, pc ProviderConfig) {
			defer wg.Done()
			providerModelData, err := s.getModels(ctx, p, pc)
			if err != nil {
				log.Printf("failed to get models from %s: %v", p, err)
				return
			}

			mu.Lock()
			combinedModelData = append(combinedModelData, providerModelData...)
			mu.Unlock()
		}(provider, providerConfig)
	}
	wg.Wait()

	sort.Slice(combinedModelData, func(i, j int) bool {
		return combinedModelData[i]["id"].(string) < combinedModelData[j]["id"].(string)
	})

	allModels := map[string]interface{}{
		"object": "list",
		"data":   combinedModelData,
	}

	// Marshal once, store the result
	respBuffer, err := json.Marshal(allModels)
	if err != nil {
		http.Error(w, `{"error":"failed to marshal models"}`, http.StatusInternalServerError)
		return
	}

	modelDataMutex.Lock()
	globalModelJSON = respBuffer // Store the JSON bytes
	modelDataMutex.Unlock()

	_, err = w.Write(respBuffer) // Write the same JSON bytes
	if err != nil {
		log.Printf("failed to write response: %v", err)
		return
	}
}

func (s *ProxyServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "<html>")
	fmt.Fprint(w, "<header><title>LLMSee</title></header>")
	fmt.Fprint(w, "<style>*{font-size:1rem}body{font-family:monospace;background-color:black;color:#c0c0c0;padding:10px}a,a:visited{text-decoration:none;color:white}a:hover{text-decoration:underline}</style>")
	fmt.Fprint(w, "<body>")
	fmt.Fprint(w, "<table>")
	fmt.Fprint(w, "<tr>")
	fmt.Fprint(w, "<td>LLMSee:</td>")
	fmt.Fprintf(w, "<td>%s</td>", VERSION)
	fmt.Fprint(w, "</tr>")
	fmt.Fprint(w, "<tr>")
	fmt.Fprint(w, "<td>WebUI:</td>")
	fmt.Fprintf(w, "<td><a href=\"http://%s/ui\">http://%s/ui</a></td>", r.Host, r.Host)
	fmt.Fprint(w, "</tr>")
	fmt.Fprint(w, "<tr>")
	fmt.Fprint(w, "<td>API Base URL:</td>")
	fmt.Fprintf(w, "<td><a href=\"http://%s/v1\">http://%s/v1</a></td>", r.Host, r.Host)
	fmt.Fprint(w, "</tr>")
	fmt.Fprint(w, "<tr>")
	fmt.Fprint(w, "<td>Model List:</td>")
	fmt.Fprintf(w, "<td><a href=\"http://%s/v1/models\">http://%s/v1/models</a></td>", r.Host, r.Host)
	fmt.Fprint(w, "</tr>")
	fmt.Fprint(w, "<tr valign=\"top\">")
	fmt.Fprint(w, "<td>Provider URLs:</td>")
	fmt.Fprint(w, "<td>")
	for provider := range s.config.Providers {
		fmt.Fprintf(w, "<div style=\"margin-bottom:5px\"><a href=\"http://%s/%s\">http://%s/%s</a></div>", r.Host, provider, r.Host, provider)
	}
	fmt.Fprint(w, "</td>")
	fmt.Fprint(w, "</tr>")
	fmt.Fprint(w, "<tr>")
	fmt.Fprint(w, "<td>Source Code:</td>")
	fmt.Fprint(w, "<td><a href=\"https://github.com/yz778/llmsee\">GitHub</a></td>")
	fmt.Fprint(w, "</tr>")
	fmt.Fprint(w, "<tr>")
	fmt.Fprint(w, "</body>")
	fmt.Fprint(w, "</html>")
}

// handleProxy processes incoming proxy requests
func (s *ProxyServer) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Handle CORS
	if s.handleCORS(w, r) {
		return
	}

	// Handle index
	if r.URL.Path == "/" {
		s.handleIndex(w, r)
		return
	}

	// Make sure body isn't too big
	r.Body = http.MaxBytesReader(w, r.Body, httpMaxRequestBodySize)
	defer r.Body.Close()

	// Get provider
	provider, remainingPath := parseProvider(r)
	v1Request := provider == "v1"

	// Special handler for /v1 router models
	if v1Request && r.Method == "GET" && remainingPath == "models" {
		s.getAllModels(w, r)
		return
	}

	// bytes -> json
	var bodyJSON map[string]interface{}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
	    if err.Error() == "http: request body too large" {
	        http.Error(w, `{"error":"Request body too large"}`, http.StatusRequestEntityTooLarge)
	    } else {
	        http.Error(w, `{"error":"Failed to read request body"}`, http.StatusInternalServerError)
	    }
	    return
	}
	if err := json.Unmarshal([]byte(bodyBytes), &bodyJSON); err != nil {
		http.Error(w, `{"error":"Failed to read request body"}`, http.StatusInternalServerError)
		return
	}

	// for /v1 requests, rewrite the model name
	if v1Request {
		if model, ok := bodyJSON["model"].(string); ok {
			parts := strings.SplitN(model, ":", 2) // <provider>:<model>
			provider = parts[0]
			model = parts[1]
			bodyJSON["model"] = model
		}
	}

	// verify the provider
	providerConfig, ok := s.config.Providers[provider]
	if !ok {
		http.Error(w, `{"error":"Invalid provider"}`, http.StatusBadRequest)
		return
	}

	// special handling for Gemini
	if providerConfig.IsGemini {
		delete(bodyJSON, "frequency_penalty")
	}

	// json -> bytes
	bodyBytes, err := json.Marshal(bodyJSON)
	if err != nil {
		http.Error(w, `{"error":"Could not mangle model"}`, http.StatusInternalServerError)
		return
	}

	// set target url
	targetURL := providerConfig.BaseURL + "/" + remainingPath
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// log initial request
	startTime := time.Now()
	reqHeadersJSON, _ := json.Marshal(r.Header)
	entry := LogEntry{
		Timestamp:       startTime.UTC().Format(time.RFC3339),
		Provider:        provider,
		Model:           bodyJSON["model"].(string),
		Method:          r.Method,
		TargetURL:       targetURL,
		RequestHeaders:  string(reqHeadersJSON),
		RequestBody:     string(bodyBytes),
		RequestBodySize: len(bodyBytes),
		UserAgent:       r.UserAgent(),
	}

	id, err := s.insertLogRequest(entry)
	if err != nil {
		log.Printf("failed to log initial request: %v", err)
	}

	entry.Id = id

	// send request to target
	ctx, cancel := context.WithTimeout(r.Context(), httpRequestTimeout)
	defer cancel()

	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, `{"error":"Failed to create proxy request"}`, http.StatusInternalServerError)
		return
	}

	proxyReq.Header = proxyHeaders(r.Header, providerConfig)

	resp, err := s.client.Do(proxyReq)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Proxy Error","message":"%s"}`, err), http.StatusInternalServerError)
		return
	}

	defer func() {
		io.Copy(io.Discard, resp.Body) // Drain the body
		resp.Body.Close()
	}()

	// Copy response headers w/o CORS
	for k, v := range resp.Header {
		if !strings.HasPrefix(k, "Access-Control") {
			w.Header()[k] = v
		}
	}

	var responseBody string
	encoding := resp.Header.Get("Content-Encoding")
	isStreaming := resp.Header.Get("Content-Type") == "text/event-stream"

	if isStreaming {
		var reader io.Reader = resp.Body
		var combinedData bytes.Buffer

		totalChunks := 0
		buf := make([]byte, 32*1024)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				totalChunks++

				// write to client
				_, err := w.Write(buf[:n])
				if err != nil {
					log.Printf("failed to write chunk: %v", err)
					break
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}

				// accumulate data for logging
				combinedData.Write(buf[:n])
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("failed to read chunk: %v", err)
				break
			}
		}

		data := combinedData.Bytes()
		if encoding != "" {
			reader, err := decompressBody(bytes.NewReader(data), encoding)
			if err != nil {
				log.Printf("failed to create decompression reader: %v", err)
			} else {
				decompressedData, err := io.ReadAll(reader)
				if err != nil {
					log.Printf("failed to decompress data: %v", err)
				} else {
					data = decompressedData
				}
			}
		}

		responseBody = processChunks(data, totalChunks)

	} else {
		// read response
		respBuffer, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, `{"error":"Failed to read response body"}`, http.StatusInternalServerError)
			return
		}

		// write response to caller
		_, err = w.Write(respBuffer)
		if err != nil {
			log.Printf("failed to write response: %v", err)
		}

		// prep response body for logging
		if encoding == "" {
			responseBody = string(respBuffer)

		} else {
			decompressedReader, err := decompressBody(bytes.NewReader(respBuffer), encoding)
			if err != nil {
				log.Printf("failed to decompress response: %v", err)
				http.Error(w, `{"error":"Failed to decompress response"}`, http.StatusInternalServerError)
				return
			}
			decompressedBody, err := io.ReadAll(decompressedReader)
			if err != nil {
				log.Printf("failed to read decompressed response: %v", err)
				http.Error(w, `{"error":"Failed to read decompressed response"}`, http.StatusInternalServerError)
				return
			}
			responseBody = string(decompressedBody)
		}
	}

	// update the request
	respHeadersJSON, _ := json.Marshal(resp.Header)
	entry.ResponseStatus = resp.StatusCode
	entry.ResponseHeaders = string(respHeadersJSON)
	entry.ResponseBody = responseBody
	entry.ResponseBodySize = len(responseBody)
	entry.DurationMs = int(time.Since(startTime).Milliseconds())
	if err := s.updateLogRequest(entry); err != nil {
		log.Printf("failed to update request log: %v", err)
	}
}
