package h2flood

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"strings"

	"github.com/conantorreswf/limithit/internal/attacks"
	"github.com/conantorreswf/limithit/internal/worker"
)

func init() {
	attacks.Register("h2flood", func() attacks.Attack { return &H2Flood{} })
}

type H2Flood struct {
	connections int
	streams     int
	method      string
	insecure    bool
}

func (h *H2Flood) Name() string { return "h2flood" }
func (h *H2Flood) Synopsis() string {
	return "HTTP/2 multiplexed stream flood over few connections"
}

func (h *H2Flood) Flags(fs *flag.FlagSet) {
	fs.IntVar(&h.connections, "connections", 1, "number of HTTP/2 connections to open")
	fs.IntVar(&h.streams, "streams", 100, "concurrent streams per connection")
	fs.StringVar(&h.method, "method", "GET", "HTTP method")
	fs.BoolVar(&h.insecure, "insecure", false, "skip TLS certificate verification")
}

func (h *H2Flood) Validate() error {
	if h.connections < 1 {
		return errors.New("connections must be >= 1")
	}
	if h.streams < 1 {
		return errors.New("streams must be >= 1")
	}
	m := strings.ToUpper(h.method)
	switch m {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		h.method = m
	default:
		return fmt.Errorf("invalid method %q", h.method)
	}
	return nil
}

func (h *H2Flood) FormFields() []attacks.FormField {
	url := ""
	connections := "1"
	streams := "100"
	total := "100"
	concurrency := "10"
	method := "GET"
	insecure := "false"
	return []attacks.FormField{
		{Flag: "url", Label: "Target URL (HTTPS for HTTP/2)", Help: "Use https:// to negotiate h2 via ALPN", Kind: attacks.FieldURL, Default: "", Value: &url},
		{Flag: "connections", Label: "HTTP/2 connections", Help: "Number of long-lived TCP connections", Kind: attacks.FieldInt, Default: "1", Validate: attacks.ValidatePosInt, Value: &connections},
		{Flag: "streams", Label: "Streams per connection", Help: "Exploits MaxConcurrentStreams gaps (try 100–1000)", Kind: attacks.FieldInt, Default: "100", Validate: attacks.ValidatePosInt, Value: &streams},
		{Flag: "total", Label: "Total requests", Kind: attacks.FieldInt, Default: "100", Validate: attacks.ValidatePosInt, Value: &total},
		{Flag: "concurrency", Label: "Concurrency (workers)", Kind: attacks.FieldInt, Default: "10", Validate: attacks.ValidatePosInt, Value: &concurrency},
		{Flag: "method", Label: "HTTP method", Kind: attacks.FieldSelect, Default: "GET", Choices: attacks.HTTPMethodChoices(), Value: &method},
		{Flag: "insecure", Label: "Skip TLS verification", Kind: attacks.FieldBool, Default: "false", Value: &insecure},
	}
}

func (h *H2Flood) Run(ctx context.Context, base attacks.Base) (attacks.Report, error) {
	// Build an HTTP/2-only transport. ForceAttemptHTTP2 negotiates h2 via ALPN
	// for HTTPS targets. For plain HTTP (h2c) targets, use HTTPS or a proxy.
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: h.insecure,
		},
		ForceAttemptHTTP2:   true,
		MaxConnsPerHost:     h.connections,
		MaxIdleConnsPerHost: h.connections,
		MaxIdleConns:        h.connections,
	}

	hc := &http.Client{
		Timeout:   base.Common.Timeout,
		Transport: tr,
	}

	method := h.method
	build := func(ctx context.Context, _ int) (*http.Request, string, error) {
		req, err := http.NewRequestWithContext(ctx, method, base.URL, nil)
		if err != nil {
			return nil, "", err
		}
		for k, vs := range base.Common.Headers {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		// Disable content-length negotiation so Go doesn't downgrade to HTTP/1.1.
		req.ContentLength = -1
		return req, "", nil
	}

	// Use streams * connections as effective concurrency so we fan out many
	// parallel requests over the limited connection pool.
	concurrency := h.streams * h.connections
	if concurrency > base.Common.Total {
		concurrency = base.Common.Total
	}

	return worker.Run(ctx, hc, build, worker.Config{
		Total:       base.Common.Total,
		Concurrency: concurrency,
		Pacer:       base.Common.Pacer,
		Tag:         "h2flood",
		Attack:      "h2flood",
		Target:      base.URL,
		ProgressCh:  base.ProgressCh,
	}), nil
}
