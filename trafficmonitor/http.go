package trafficmonitor

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

//go:embed web/*
var webFiles embed.FS

func (m *Monitor) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/summary", m.handleSummary)
	mux.HandleFunc("GET /api/timeseries", m.handleTimeSeries)
	mux.HandleFunc("GET /api/traffic", m.handleAggregate)
	mux.HandleFunc("GET /api/direct-candidates", m.handleDirectCandidates)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	assets, err := fs.Sub(webFiles, "web")
	if err != nil {
		mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "内置历史流量面板资源不可用"})
		})
		return securityHeaders(mux)
	}
	mux.Handle("/", http.FileServer(http.FS(assets)))
	return securityHeaders(mux)
}

func (m *Monitor) handleSummary(w http.ResponseWriter, r *http.Request) {
	result, err := m.summary(r.Context(), parseReportQuery(r, 100))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (m *Monitor) handleAggregate(w http.ResponseWriter, r *http.Request) {
	result, err := m.aggregate(r.Context(), parseReportQuery(r, 100))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (m *Monitor) handleTimeSeries(w http.ResponseWriter, r *http.Request) {
	result, err := m.timeSeries(r.Context(), parseReportQuery(r, 100))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (m *Monitor) handleDirectCandidates(w http.ResponseWriter, r *http.Request) {
	minutes := parseInt(r.URL.Query().Get("minutes"), 1440)
	limit := parseInt(r.URL.Query().Get("limit"), 200)
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	result, err := m.directCandidates(r.Context(), minutes, limit, search)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func parseInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseReportQuery(r *http.Request, defaultLimit int) AggregateQuery {
	return AggregateQuery{
		Dimension: r.URL.Query().Get("dimension"),
		Minutes:   parseInt(r.URL.Query().Get("minutes"), 1440),
		Limit:     parseInt(r.URL.Query().Get("limit"), defaultLimit),
		Route:     r.URL.Query().Get("route"),
		Search:    strings.TrimSpace(r.URL.Query().Get("search")),
		Sort:      r.URL.Query().Get("sort"),
		Order:     r.URL.Query().Get("order"),
	}
}

func (m *Monitor) String() string {
	return fmt.Sprintf("historical traffic monitor (%s)", m.DashboardURL())
}
