package main

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
)

// handleLogList serves the UI data based on the page query
func (s *ProxyServer) handleLogList(w http.ResponseWriter, r *http.Request) {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil {
		page = 1
	}

	perPage := s.config.PageSize
	logs, totalLogs, err := s.getLogEntries(page, perPage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalPages := int(math.Ceil(float64(totalLogs) / float64(perPage)))
	
	// Adjust page if out of bounds
	if page < 1 || (totalPages > 0 && page > totalPages) {
		page = max(1, totalPages)
		logs, totalLogs, err = s.getLogEntries(page, perPage)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	response := LogResponse{
		Logs:        logs,
		TotalPages:  totalPages,
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

	entry, err := s.getLogDetail(int64(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entry)
}
