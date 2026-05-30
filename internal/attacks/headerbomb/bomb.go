package headerbomb

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/conantorreswf/limithit/internal/attacks"
	"github.com/conantorreswf/limithit/internal/worker"
)

func init() {
	attacks.Register("headerbomb", func() attacks.Attack { return &Headerbomb{} })
}

type Headerbomb struct {
	method      string
	headerCount int
	headerSize  int
	bodyStart   int
	bodyMax     int
	bodyStep    int
}

func (h *Headerbomb) Name() string     { return "headerbomb" }
func (h *Headerbomb) Synopsis() string { return "oversized headers and progressively growing body" }

func (h *Headerbomb) Flags(fs *flag.FlagSet) {
	fs.StringVar(&h.method, "method", "", "HTTP method (default POST if body>0 else GET)")
	fs.IntVar(&h.headerCount, "header-count", 500, "X-Junk headers per request")
	fs.IntVar(&h.headerSize, "header-size", 1024, "bytes per junk header value")
	fs.IntVar(&h.bodyStart, "body-start", 1024, "initial body size (bytes)")
	fs.IntVar(&h.bodyMax, "body-max", 16<<20, "max body size (bytes)")
	fs.IntVar(&h.bodyStep, "body-step", 0, "body growth step (0 = double each time)")
}

func (h *Headerbomb) Validate() error {
	if h.headerCount < 0 || h.headerSize < 0 {
		return fmt.Errorf("headerbomb: count and size must be >= 0")
	}
	if h.method != "" {
		m, err := validateMethod(h.method)
		if err != nil {
			return err
		}
		h.method = m
	}
	return nil
}

func (h *Headerbomb) FormFields() []attacks.FormField {
	url := ""
	total := "50"
	concurrency := "5"
	method := ""
	timeout := "15"
	headerCount := "500"
	headerSize := "1024"
	bodyStart := "1024"
	bodyMax := "16777216"
	bodyStep := "0"
	methodChoices := append([]string{"auto (POST if body > 0, else GET)="}, attacks.HTTPMethodChoices()...)
	return []attacks.FormField{
		{Flag: "url", Label: "Target URL", Kind: attacks.FieldURL, Default: "", Value: &url},
		{Flag: "total", Label: "Total requests", Kind: attacks.FieldInt, Default: "50", Validate: attacks.ValidatePosInt, Value: &total},
		{Flag: "concurrency", Label: "Concurrency (workers)", Kind: attacks.FieldInt, Default: "5", Validate: attacks.ValidatePosInt, Value: &concurrency},
		{Flag: "method", Label: "HTTP method", Kind: attacks.FieldSelect, Default: "", Choices: methodChoices, Value: &method},
		{Flag: "timeout", Label: "Timeout (s)", Kind: attacks.FieldInt, Default: "15", Validate: attacks.ValidatePosInt, Value: &timeout},
		{Flag: "header-count", Label: "Junk header count", Kind: attacks.FieldInt, Default: "500", Validate: attacks.ValidatePosInt, Value: &headerCount},
		{Flag: "header-size", Label: "Bytes per junk header", Kind: attacks.FieldInt, Default: "1024", Validate: attacks.ValidatePosInt, Value: &headerSize},
		{Flag: "body-start", Label: "Initial body size (bytes)", Kind: attacks.FieldInt, Default: "1024", Validate: attacks.ValidatePosInt, Value: &bodyStart},
		{Flag: "body-max", Label: "Max body size (bytes)", Kind: attacks.FieldInt, Default: "16777216", Validate: attacks.ValidatePosInt, Value: &bodyMax},
		{Flag: "body-step", Label: "Body growth step (0 = double)", Kind: attacks.FieldInt, Default: "0", Validate: attacks.ValidateNonNegInt, Value: &bodyStep},
	}
}

func (h *Headerbomb) Run(ctx context.Context, base attacks.Base) (attacks.Report, error) {
	bodyMax := h.bodyMax
	if bodyMax < h.bodyStart {
		bodyMax = h.bodyStart
	}

	junkVal := strings.Repeat("A", h.headerSize)
	method := h.method
	if method == "" {
		if bodyMax > 0 {
			method = http.MethodPost
		} else {
			method = http.MethodGet
		}
	}

	var currentBody atomic.Int64
	currentBody.Store(int64(h.bodyStart))

	bodyStep := h.bodyStep

	build := func(ctx context.Context, _ int) (*http.Request, string, error) {
		bodySize := int(currentBody.Load())
		if bodySize < bodyMax {
			step := bodyStep
			if step <= 0 {
				step = bodySize
				if step == 0 {
					step = 1024
				}
			}
			next := bodySize + step
			if next > bodyMax {
				next = bodyMax
			}
			currentBody.CompareAndSwap(int64(bodySize), int64(next))
		}

		var req *http.Request
		var err error
		if bodySize > 0 {
			body := bytes.NewReader(bytes.Repeat([]byte{'A'}, bodySize))
			req, err = http.NewRequestWithContext(ctx, method, base.URL, body)
		} else {
			req, err = http.NewRequestWithContext(ctx, method, base.URL, nil)
		}
		if err != nil {
			return nil, "", err
		}
		for k, vs := range base.Common.Headers {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		for i := 0; i < h.headerCount; i++ {
			req.Header.Add(fmt.Sprintf("X-Junk-%d", i), junkVal)
		}
		return req, "", nil
	}

	tag := fmt.Sprintf("headerbomb (hdrs=%dx%dB body=%d→%dB)",
		h.headerCount, h.headerSize, h.bodyStart, bodyMax)
	report := worker.Run(ctx, base.Client, build, worker.Config{
		Total:       base.Common.Total,
		Concurrency: base.Common.Concurrency,
		Tag:         tag,
		Attack:      "headerbomb",
		Target:      base.URL,
		ProgressCh:  base.ProgressCh,
	})
	return report, nil
}

func validateMethod(m string) (string, error) {
	m = strings.ToUpper(m)
	switch m {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return m, nil
	}
	return "", fmt.Errorf("invalid method %q", m)
}
