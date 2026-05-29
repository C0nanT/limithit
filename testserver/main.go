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

	"github.com/conantorreswf/limithit/testserver/handler"
	"github.com/conantorreswf/limithit/testserver/ratelimit"
	"github.com/conantorreswf/limithit/testserver/store"
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

// offenderAdapter exposes ratelimit.Registry top offenders as store.OffenderSource.
type offenderAdapter struct{ reg *ratelimit.Registry }

func (a offenderAdapter) TopOffenders(n int) []store.IPStat {
	src := a.reg.TopOffenders(n)
	out := make([]store.IPStat, 0, len(src))
	for _, s := range src {
		out = append(out, store.IPStat{IP: s.Key, Denied: s.Denied})
	}
	return out
}

func main() {
	port := flag.Int("port", envInt("PORT", 8080), "listen port")
	rate := flag.Float64("rate", envFloat("RATE_LIMIT_RPS", 0), "per-IP rate limit in req/s (0 = disabled)")
	burst := flag.Float64("burst", 0, "per-IP burst size (defaults to --rate)")
	trustXFF := flag.String("trust-xff-cidr", "", `comma-separated CIDRs of trusted proxies whose XFF will be honoured (e.g. "127.0.0.1/8,10.0.0.0/8")`)
	authUser := flag.String("auth-user", "admin", "auth handler username")
	authPass := flag.String("auth-pass", "changeme", "auth handler password")
	flag.Parse()

	if *burst == 0 {
		*burst = *rate
	}

	trusted, err := ratelimit.ParseCIDRList(*trustXFF)
	if err != nil {
		log.Fatalf("invalid --trust-xff-cidr: %v", err)
	}
	if len(trusted) == 0 {
		log.Printf("XFF trust list empty — X-Forwarded-For headers ignored")
	} else {
		log.Printf("trusted proxy CIDRs: %v", trusted)
	}

	s := store.New()
	broadcaster := store.NewBroadcaster()

	var registry *ratelimit.Registry
	if *rate > 0 {
		registry = ratelimit.NewRegistry(*rate, *burst)
		defer registry.Close()
		s.SetOffenderSource(offenderAdapter{reg: registry})
		log.Printf("per-IP rate limiting: %.0f req/s, burst %.0f", *rate, *burst)
	}

	mux := http.NewServeMux()

	// dashboard + SSE (no rate limiting, no recording)
	mux.HandleFunc("/", handler.DashboardHandler)
	mux.Handle("/metrics", handler.SSEHandler(broadcaster))

	limited := func(h http.Handler) http.Handler {
		return handler.RecordingMiddleware(s, ratelimit.Middleware(registry, trusted, h))
	}
	mux.Handle("/api/ping", limited(http.HandlerFunc(handler.PingHandler)))
	mux.Handle("/api/echo", limited(http.HandlerFunc(handler.EchoHandler)))
	mux.Handle("/api/auth", limited(handler.NewAuthHandler(*authUser, *authPass, trusted)))

	// background broadcaster tick
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			broadcaster.Broadcast(s.Snapshot())
		}
	}()

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: mux,

		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second, // mitigate slowloris
		IdleTimeout:       30 * time.Second,
		// WriteTimeout must be 0 so SSE streams stay open
		MaxHeaderBytes: 16 << 10, // 16KB — bar headerbomb
	}

	log.Printf("testserver listening on http://localhost:%d", *port)
	log.Printf("  GET  http://localhost:%d/api/ping", *port)
	log.Printf("  POST http://localhost:%d/api/echo", *port)
	log.Printf("  POST http://localhost:%d/api/auth", *port)
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
