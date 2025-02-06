package main

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
)

// handleLogList serves the UI data based on the page query
func (s *ProxyServer) handleLogList(w http.ResponseWriter, r *http.Request) {
	var totalLogs int
	err := s.db.QueryRow("SELECT COUNT(*) FROM logs").Scan(&totalLogs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalPages := totalLogs / s.config.PageSize
	perPage := s.config.PageSize

	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil {
		page = 1
	}

	if page < 1 || page > totalPages {
		page = totalPages
	}

	page = max(1, page)
	offset := (page - 1) * perPage

	if offset > totalLogs {
		page = max(1, totalPages)
		offset = (page - 1) * perPage
	}

	rows, err := s.db.Query(`
		SELECT
			id,
			timestamp,
			provider,
			method,
			model,
			length(request_body) AS request_body_size,
			length(response_body) AS response_body_size,
			response_status,
			useragent,
			duration_ms
		FROM logs
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`, perPage, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []LogEntry
	for rows.Next() {
		var entry LogEntry
		err := rows.Scan(
			&entry.Id,
			&entry.Timestamp,
			&entry.Provider,
			&entry.Method,
			&entry.Model,
			&entry.RequestBodySize,
			&entry.ResponseBodySize,
			&entry.ResponseStatus,
			&entry.UserAgent,
			&entry.DurationMs,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		logs = append(logs, entry)
	}

	response := LogResponse{
		Logs:        logs,
		TotalPages:  int(math.Ceil(float64(totalLogs) / float64(perPage))),
		CurrentPage: page,
		TotalLogs:   totalLogs,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleLogDetail serves the detail of a specific log entry
func (s *ProxyServer) handleLogDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id < 0 {
		http.Error(w, `{"error":"Invalid log ID"}`, http.StatusBadRequest)
		return
	}

	var entry LogEntry
	err = s.db.QueryRow(`
		SELECT
			target_url,
			model,
			request_headers,
			request_body,
			response_status,
			response_headers,
			response_body
		FROM logs
		WHERE id = ?
		`, id).Scan(
		&entry.TargetURL,
		&entry.Model,
		&entry.RequestHeaders,
		&entry.RequestBody,
		&entry.ResponseStatus,
		&entry.ResponseHeaders,
		&entry.ResponseBody,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entry)
}
