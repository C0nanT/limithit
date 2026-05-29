package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/conantorreswf/limithit/internal/metrics"
)

// Table writes the human-readable summary to w.
func Table(w io.Writer, r *metrics.Report) {
	fmt.Fprint(w, r.String())
}

// JSON writes indented JSON to w.
func JSON(w io.Writer, r *metrics.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// CSV writes per-second bucket rows to w. One row per second offset.
func CSV(w io.Writer, r *metrics.Report) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"second_offset", "sent", "ok", "rate_limited", "errors"}); err != nil {
		return err
	}
	for _, b := range r.PerSecond {
		if err := cw.Write([]string{
			strconv.Itoa(b.SecondOffset),
			strconv.FormatInt(b.Sent, 10),
			strconv.FormatInt(b.OK, 10),
			strconv.FormatInt(b.RateLimited, 10),
			strconv.FormatInt(b.Errors, 10),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// Thresholds configure regression detection for Compare.
type Thresholds struct {
	P99PctIncrease  float64 // flag regression if p99 grows by more than this % (default 10)
	R429PctDecrease float64 // flag regression if 429 ratio shrinks by more than this % (default 10)
}

// Regression describes a single detected regression.
type Regression struct {
	Field     string
	Baseline  float64
	Current   float64
	PctChange float64
	Message   string
}

// Compare diffs current against baseline and returns detected regressions.
func Compare(baseline, current *metrics.Report, t Thresholds) []Regression {
	if t.P99PctIncrease <= 0 {
		t.P99PctIncrease = 10.0
	}
	if t.R429PctDecrease <= 0 {
		t.R429PctDecrease = 10.0
	}

	var out []Regression

	if baseline.Latency.P99 > 0 {
		base := float64(baseline.Latency.P99.Milliseconds())
		curr := float64(current.Latency.P99.Milliseconds())
		pct := (curr - base) / base * 100
		if pct > t.P99PctIncrease {
			out = append(out, Regression{
				Field:     "p99_latency",
				Baseline:  base,
				Current:   curr,
				PctChange: pct,
				Message:   fmt.Sprintf("p99 latency up %.1f%% (%.0fms → %.0fms)", pct, base, curr),
			})
		}
	}

	if baseline.Sent > 0 && current.Sent > 0 {
		baseRatio := float64(baseline.TooMany) / float64(baseline.Sent) * 100
		currRatio := float64(current.TooMany) / float64(current.Sent) * 100
		if baseRatio > 0 {
			pct := (currRatio - baseRatio) / baseRatio * 100
			if pct < -t.R429PctDecrease {
				out = append(out, Regression{
					Field:     "rate_limit_ratio",
					Baseline:  baseRatio,
					Current:   currRatio,
					PctChange: pct,
					Message: fmt.Sprintf("429 ratio dropped %.1f%% (%.1f%% → %.1f%%)",
						-pct, baseRatio, currRatio),
				})
			}
		}
	}

	return out
}

// JSONAny writes indented JSON for any value to w.
func JSONAny(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// LoadBaseline reads a JSON-encoded *metrics.Report from path.
func LoadBaseline(path string) (*metrics.Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var r metrics.Report
	if err := json.NewDecoder(f).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode baseline %q: %w", path, err)
	}
	return &r, nil
}
