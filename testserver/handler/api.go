package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/conantorreswf/limithit/testserver/store"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (rr *responseRecorder) WriteHeader(status int) {
	rr.status = status
	rr.ResponseWriter.WriteHeader(status)
}

func RecordingMiddleware(s *store.MetricsStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rr := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rr, r)
		s.Record(r.Method, r.URL.Path, rr.status, time.Since(start))
	})
}

func PingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"pong":true}`))
}

func EchoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"failed to read body"}`))
		return
	}

	// validate JSON or wrap raw bytes
	var v any
	w.Header().Set("Content-Type", "application/json")
	if json.Unmarshal(body, &v) == nil {
		w.Write(body)
	} else {
		resp, _ := json.Marshal(map[string]string{"echo": string(body)})
		w.Write(resp)
	}
}
