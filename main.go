package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/conantorreswf/ratelash/internal/client"
	"github.com/conantorreswf/ratelash/internal/worker"
)

type headerFlag struct {
	headers http.Header
}

func (h *headerFlag) String() string {
	if h.headers == nil {
		return ""
	}
	parts := make([]string, 0, len(h.headers))
	for k, vs := range h.headers {
		for _, v := range vs {
			parts = append(parts, fmt.Sprintf("%s: %s", k, v))
		}
	}
	return strings.Join(parts, ", ")
}

func (h *headerFlag) Set(s string) error {
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return errors.New(`header must be in "Key: Value" format`)
	}
	key := strings.TrimSpace(s[:idx])
	val := strings.TrimSpace(s[idx+1:])
	if key == "" {
		return errors.New("header key is empty")
	}
	if h.headers == nil {
		h.headers = make(http.Header)
	}
	h.headers.Add(key, val)
	return nil
}

var allowedMethods = map[string]struct{}{
	"GET": {}, "POST": {}, "PUT": {}, "PATCH": {}, "DELETE": {}, "HEAD": {},
}

func main() {
	var (
		rawURL      string
		total       int
		concurrency int
		method      string
		timeoutSec  int
		body        string
		hdr         headerFlag
	)

	flag.StringVar(&rawURL, "url", "", "target URL (required)")
	flag.IntVar(&total, "total", 100, "total number of requests")
	flag.IntVar(&concurrency, "concurrency", 10, "number of concurrent workers")
	flag.StringVar(&method, "method", "GET", "HTTP method (GET, POST, PUT, PATCH, DELETE, HEAD)")
	flag.IntVar(&timeoutSec, "timeout", 10, "per-request timeout in seconds")
	flag.StringVar(&body, "body", "", "request body (used with POST/PUT/PATCH)")
	flag.Var(&hdr, "header", `custom header in "Key: Value" form (repeatable)`)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "ratelash — HTTP rate-limit probe\n\nUsage:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if err := validate(rawURL, total, concurrency, timeoutSec, &method); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n\n", err)
		flag.Usage()
		os.Exit(1)
	}

	if concurrency > total {
		concurrency = total
	}

	cfg := client.Config{
		URL:     rawURL,
		Method:  method,
		Body:    body,
		Headers: hdr.headers,
		Timeout: time.Duration(timeoutSec) * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	report := worker.Run(ctx, cfg, total, concurrency)
	fmt.Print(report.String())

	if ctx.Err() != nil {
		fmt.Fprintln(os.Stderr, "\n(interrupted by signal — totals reflect requests sent before cancellation)")
	}
}

func validate(rawURL string, total, concurrency, timeoutSec int, method *string) error {
	if rawURL == "" {
		return errors.New("--url is required")
	}
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return fmt.Errorf("invalid --url: %w", err)
	}
	if total <= 0 {
		return errors.New("--total must be > 0")
	}
	if concurrency <= 0 {
		return errors.New("--concurrency must be > 0")
	}
	if timeoutSec <= 0 {
		return errors.New("--timeout must be > 0")
	}
	*method = strings.ToUpper(*method)
	if _, ok := allowedMethods[*method]; !ok {
		return fmt.Errorf("invalid --method %q", *method)
	}
	return nil
}
