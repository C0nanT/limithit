package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/conantorreswf/ratelash/testserver/handler"
	"github.com/conantorreswf/ratelash/testserver/ratelimit"
	"github.com/conantorreswf/ratelash/testserver/store"
)

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func main() {
	port := flag.Int("port", envInt("PORT", 8080), "listen port")
	rate := flag.Float64("rate", envFloat("RATE_LIMIT_RPS", 0), "rate limit in req/s (0 = disabled)")
	burst := flag.Float64("burst", 0, "burst size (defaults to --rate value)")
	flag.Parse()

	if *burst == 0 {
		*burst = *rate
	}

	s := store.New()
	broadcaster := store.NewBroadcaster()

	var limiter *ratelimit.Limiter
	if *rate > 0 {
		limiter = ratelimit.New(*rate, *burst)
		log.Printf("rate limiting enabled: %.0f req/s, burst %.0f", *rate, *burst)
	}

	mux := http.NewServeMux()

	// dashboard + SSE (no rate limiting, no recording)
	mux.HandleFunc("/", handler.DashboardHandler)
	mux.Handle("/metrics", handler.SSEHandler(broadcaster))

	// API endpoints: rate limit middleware wraps recording middleware wraps handler
	pingHandler := handler.RecordingMiddleware(s,
		ratelimit.Middleware(limiter, http.HandlerFunc(handler.PingHandler)))
	echoHandler := handler.RecordingMiddleware(s,
		ratelimit.Middleware(limiter, http.HandlerFunc(handler.EchoHandler)))

	mux.Handle("/api/ping", pingHandler)
	mux.Handle("/api/echo", echoHandler)

	// background broadcaster tick
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			broadcaster.Broadcast(s.Snapshot())
		}
	}()

	srv := &http.Server{
		Addr:        fmt.Sprintf(":%d", *port),
		Handler:     mux,
		ReadTimeout: 10 * time.Second,
		// WriteTimeout must be 0 for SSE connections to stay open
	}

	log.Printf("testserver listening on http://localhost:%d", *port)
	log.Printf("  GET  http://localhost:%d/api/ping", *port)
	log.Printf("  POST http://localhost:%d/api/echo", *port)
	log.Printf("  dashboard: http://localhost:%d/", *port)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
}
