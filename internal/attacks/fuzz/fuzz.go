package fuzz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/conantorreswf/ratelash/internal/client"
	"github.com/conantorreswf/ratelash/internal/metrics"
	"github.com/conantorreswf/ratelash/internal/worker"
)

type Options struct {
	BaseURL     string // scheme://host[:port] — no path
	WordlistPth string // optional file override
	CacheBust   bool
	Headers     http.Header
	Timeout     time.Duration
	Total       int
	Concurrency int
}

func Run(ctx context.Context, opts Options) (*metrics.Report, error) {
	u, err := url.Parse(opts.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("fuzz: invalid base url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("fuzz: base url must include scheme + host")
	}
	base := strings.TrimRight(u.Scheme+"://"+u.Host, "/")

	var wl *metrics.Wordlist
	if opts.WordlistPth != "" {
		wl, err = metrics.LoadWordlist(opts.WordlistPth)
		if err != nil {
			return nil, fmt.Errorf("fuzz: %w", err)
		}
	} else {
		wl = metrics.DefaultWordlist()
	}
	if wl.Size() == 0 {
		return nil, fmt.Errorf("fuzz: wordlist is empty")
	}

	hc := client.New(client.Config{Timeout: opts.Timeout}, opts.Concurrency)

	build := func(ctx context.Context, _ int) (*http.Request, string, error) {
		path := wl.Next()
		full := base + path
		if opts.CacheBust {
			var buf [8]byte
			_, _ = rand.Read(buf[:])
			sep := "?"
			if strings.Contains(path, "?") {
				sep = "&"
			}
			full = full + sep + "_cb=" + hex.EncodeToString(buf[:])
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
		if err != nil {
			return nil, path, err
		}
		for k, vs := range opts.Headers {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		return req, path, nil
	}

	tag := fmt.Sprintf("fuzz (paths=%d cache-bust=%v)", wl.Size(), opts.CacheBust)
	report := worker.Run(ctx, hc, build, worker.Config{
		Total:       opts.Total,
		Concurrency: opts.Concurrency,
		Tag:         tag,
	})
	return report, nil
}
