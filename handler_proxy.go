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
func processChunks(chunks [][]byte) string {
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
	totalChunks := len(chunks)

	for _, chunk := range chunks {
		lines := strings.Split(string(chunk), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "data: ") {
				isOpenAISpec = true
				data := strings.TrimPrefix(line, "data: ")
				data = strings.TrimSpace(data)
				if data == "[DONE]" {
					response := Response{
						Metadata:    metadata,
						Content:     strings.TrimSpace(assembledContent.String()),
						Usage:       &usageStats,
						TotalChunks: totalChunks,
					}
					jsonResponse, _ := json.MarshalIndent(response, "", "  ")
					return string(jsonResponse)
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
	}

	if isOpenAISpec {
		response := Response{
			Metadata:    metadata,
			Content:     strings.TrimSpace(assembledContent.String()),
			Usage:       &usageStats,
			TotalChunks: totalChunks,
		}
		jsonResponse, _ := json.MarshalIndent(response, "", "  ")
		return string(jsonResponse)

	} else {
		// Concatenate all chunks and return as a single string
		var allChunksContent strings.Builder
		for _, chunk := range chunks {
			allChunksContent.Write(chunk)
		}
		return allChunksContent.String()
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

func (s *ProxyServer) getModels(ctx context.Context, provider string, providerConfig ProviderConfig) ([]map[string]interface{}, error) {
	targetURL := providerConfig.BaseURL + "/models"

	proxyReq, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for provider %s: %w", provider, err)
	}

	proxyReq.Header = proxyHeaders(http.Header{}, providerConfig)

	resp, err := s.client.Do(proxyReq)
	if err != nil {
		return nil, fmt.Errorf("proxy request failed for provider %s: %w", provider, err)
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	respBuffer, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body for provider %s: %w", provider, err)
	}

	encoding := resp.Header.Get("Content-Encoding")
	if encoding != "" {
		decompressedReader, err := decompressBody(bytes.NewReader(respBuffer), encoding)
		if err != nil {
			return nil, fmt.Errorf("error decompressing response for provider %s: %w", provider, err)
		}
		decompressedBody, err := io.ReadAll(decompressedReader)
		if err != nil {
			return nil, fmt.Errorf("error reading decompressed response for provider %s: %w", provider, err)
		}
		respBuffer = decompressedBody
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(respBuffer, &parsed); err != nil {
		return nil, fmt.Errorf("could not parse response for provider %s: %w", provider, err)
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
			log.Printf("Error writing cached response: %v", err)
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
				log.Printf("Error fetching models from %s: %v", p, err)
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
		http.Error(w, `{"error":"Could not marshal models"}`, http.StatusInternalServerError)
		return
	}

	modelDataMutex.Lock()
	globalModelJSON = respBuffer // Store the JSON bytes
	modelDataMutex.Unlock()

	_, err = w.Write(respBuffer) // Write the same JSON bytes
	if err != nil {
		log.Printf("Error writing response: %v", err)
		return
	}
}

// handleProxy processes incoming proxy requests
func (s *ProxyServer) handleProxy(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><header><title>LLMSee</title><style>body{font-family:monospace;background-color:black;color:white;padding:10px}a,a:visited{color:yellow}</style></header><body>")
		fmt.Fprintf(w, "<div>LLMSee %s Ready</div>", VERSION)
		fmt.Fprint(w, "<div style=\"margin-top:15px\">WebUI:</div>")
		fmt.Fprintf(w, "<div><a href=\"http://%s/ui\">http://%s/ui</a></div>", r.Host, r.Host)
		fmt.Fprint(w, "<div style=\"margin-top:15px\">API Base URL:</div>")
		fmt.Fprintf(w, "<div><a href=\"http://%s/v1\">http://%s/v1</a>", r.Host, r.Host)
		fmt.Fprint(w, "<div style=\"margin-top:15px\">Individual Provider URLs:</div>")
		for provider := range s.config.Providers {
			fmt.Fprintf(w, "<div><a href=\"http://%s/%s\">http://%s/%s</a></div>", r.Host, provider, r.Host, provider)
		}
		fmt.Fprint(w, "<div style=\"margin-top:15px\">Source Code:</div>")
		fmt.Fprint(w, "<div><a href=\"https://github.com/yz778/llmsee\">GitHub</a></div>")
		fmt.Fprint(w, "</body></html>")

		return
	}

	// Handle CORS
	if s.handleCORS(w, r) {
		return
	}

	// Make sure body isn't too big
	r.Body = http.MaxBytesReader(w, r.Body, httpMaxRequestBodySize)

	// Process provider
	provider, remainingPath := parseProvider(r)

	// Special handler for /v1 router
	if provider == "v1" {
		if r.Method == "GET" && remainingPath == "models" {
			s.getAllModels(w, r)
			return

		} else {
			// rewrite the body with modified model
			var parsed map[string]interface{}
			bodyBytes, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal([]byte(bodyBytes), &parsed); err != nil {
				http.Error(w, `404 page not found (API request missing)`, http.StatusNotFound)
				return
			}

			if model, ok := parsed["model"].(string); ok {
				parts := strings.SplitN(model, ":", 2) // <provider>:<model>
				provider = parts[0]
				model = parts[1]
				parsed["model"] = model
				bodyBytes, err := json.Marshal(parsed)
				if err != nil {
					http.Error(w, `{"error":"Could not mangle model"}`, http.StatusInternalServerError)
					return
				}
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}
		}
	}

	providerConfig, ok := s.config.Providers[provider]
	if !ok {
		http.Error(w, `{"error":"Invalid provider"}`, http.StatusBadRequest)
		return
	}

	targetURL := providerConfig.BaseURL + "/" + remainingPath
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// Process request body
	var bodyBuffer []byte
	var err error
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		bodyBuffer, err = io.ReadAll(io.LimitReader(r.Body, httpMaxRequestBodySize))
		if err != nil {
			if err.Error() == "http: request body too large" {
				http.Error(w, `{"error":"Request body too large"}`, http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, `{"error":"Failed to read request body"}`, http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()
	}

	startTime := time.Now()
	reqHeadersJSON, _ := json.Marshal(r.Header)
	entry := LogEntry{
		Timestamp:       startTime.UTC().Format(time.RFC3339),
		Provider:        provider,
		Method:          r.Method,
		TargetURL:       targetURL,
		RequestHeaders:  string(reqHeadersJSON),
		RequestBody:     string(bodyBuffer),
		RequestBodySize: len(bodyBuffer),
		UserAgent:       r.UserAgent(),
	}

	// log the request
	id, model, err := s.insertLogRequest(entry)
	if err != nil {
		log.Printf("Error logging initial request: %v", err)
	}
	entry.Id = id
	entry.Model = model

	ctx, cancel := context.WithTimeout(r.Context(), httpRequestTimeout)
	defer cancel()

	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, targetURL, strings.NewReader(string(bodyBuffer)))
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

	isStreaming := r.URL.Query().Get("stream") == "true"
	if bodyBuffer != nil {
		var reqJSON map[string]interface{}
		if err := json.Unmarshal(bodyBuffer, &reqJSON); err == nil {
			if streamVal, ok := reqJSON["stream"].(bool); ok {
				isStreaming = isStreaming || streamVal
			}
		}
	}

	// Copy response headers, skip for CORS
	for k, v := range resp.Header {
		if !strings.HasPrefix(k, "Access-Control") {
			w.Header()[k] = v
		}
	}
	w.WriteHeader(resp.StatusCode)

	var responseBody string
	encoding := resp.Header.Get("Content-Encoding")

	if isStreaming {
		var reader io.Reader = resp.Body

		if encoding != "" {
			decompressedReader, err := decompressBody(resp.Body, encoding)
			if err != nil {
				log.Printf("Error decompressing streaming response: %v", err)
				http.Error(w, `{"error":"Failed to decompress streaming response"}`, http.StatusInternalServerError)
				return
			}
			reader = decompressedReader
		}

		var chunks [][]byte
		buf := make([]byte, 32*1024)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				chunks = append(chunks, chunk)
				_, err := w.Write(chunk)
				if err != nil {
					log.Printf("Error writing chunk: %v", err)
					break
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("Error reading chunk: %v", err)
				break
			}
		}
		responseBody = processChunks(chunks)

	} else {
		respBuffer, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, `{"error":"Failed to read response body"}`, http.StatusInternalServerError)
			return
		}

		// Handle compressed responses
		if encoding == "" {
			responseBody = string(respBuffer)

		} else {
			decompressedReader, err := decompressBody(bytes.NewReader(respBuffer), encoding)
			if err != nil {
				log.Printf("Error decompressing response: %v", err)
				http.Error(w, `{"error":"Failed to decompress response"}`, http.StatusInternalServerError)
				return
			}
			decompressedBody, err := io.ReadAll(decompressedReader)
			if err != nil {
				log.Printf("Error reading decompressed response: %v", err)
				http.Error(w, `{"error":"Failed to read decompressed response"}`, http.StatusInternalServerError)
				return
			}
			responseBody = string(decompressedBody)
		}

		// Write original response to client
		_, err = w.Write(respBuffer)
		if err != nil {
			log.Printf("Error writing response: %v", err)
			return
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
		log.Printf("Error updating request log: %v", err)
	}
}
