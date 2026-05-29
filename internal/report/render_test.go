package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/conantorreswf/limithit/internal/metrics"
)

func makeReport(sent, success, tooMany int, p99ms int64) *metrics.Report {
	return &metrics.Report{
		Sent:    sent,
		Success: success,
		TooMany: tooMany,
		Latency: metrics.LatencyStats{
			P99: time.Duration(p99ms) * time.Millisecond,
		},
		StatusCounts: map[int]int{200: success, 429: tooMany},
		PerSecond: []metrics.Bucket{
			{SecondOffset: 0, Sent: int64(sent / 2), OK: int64(success / 2), RateLimited: int64(tooMany / 2)},
			{SecondOffset: 1, Sent: int64(sent / 2), OK: int64(success / 2), RateLimited: int64(tooMany / 2)},
		},
	}
}

func TestJSON_roundtrip(t *testing.T) {
	r := makeReport(100, 60, 40, 50)
	var buf bytes.Buffer
	if err := JSON(&buf, r); err != nil {
		t.Fatalf("JSON: %v", err)
	}

	var decoded metrics.Report
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Sent != r.Sent {
		t.Errorf("sent: want %d got %d", r.Sent, decoded.Sent)
	}
	if decoded.Latency.P99 != r.Latency.P99 {
		t.Errorf("p99: want %s got %s", r.Latency.P99, decoded.Latency.P99)
	}
}

func TestJSON_nonEmpty(t *testing.T) {
	r := makeReport(200, 150, 50, 120)
	var buf bytes.Buffer
	if err := JSON(&buf, r); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if !strings.Contains(buf.String(), `"sent"`) {
		t.Error("JSON output missing 'sent' field")
	}
	if !strings.Contains(buf.String(), `"latency"`) {
		t.Error("JSON output missing 'latency' field")
	}
}

func TestCSV_headers(t *testing.T) {
	r := makeReport(100, 60, 40, 50)
	var buf bytes.Buffer
	if err := CSV(&buf, r); err != nil {
		t.Fatalf("CSV: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < 1 {
		t.Fatal("CSV empty")
	}
	if !strings.HasPrefix(lines[0], "second_offset") {
		t.Errorf("CSV header wrong: %q", lines[0])
	}
	// 2 buckets + header = 3 lines
	if len(lines) != 3 {
		t.Errorf("CSV line count: want 3 got %d", len(lines))
	}
}

func TestCompare_noRegression(t *testing.T) {
	baseline := makeReport(100, 60, 40, 50)
	current := makeReport(100, 60, 40, 52) // p99 up only 4%
	regressions := Compare(baseline, current, Thresholds{P99PctIncrease: 10, R429PctDecrease: 10})
	if len(regressions) != 0 {
		t.Errorf("expected no regressions, got %v", regressions)
	}
}

func TestCompare_p99Regression(t *testing.T) {
	baseline := makeReport(100, 60, 40, 50)
	current := makeReport(100, 60, 40, 100) // p99 doubled = 100% increase
	regressions := Compare(baseline, current, Thresholds{P99PctIncrease: 10, R429PctDecrease: 10})
	if len(regressions) == 0 {
		t.Fatal("expected p99 regression")
	}
	if regressions[0].Field != "p99_latency" {
		t.Errorf("wrong field: %s", regressions[0].Field)
	}
}

func TestCompare_429Regression(t *testing.T) {
	baseline := makeReport(100, 60, 40, 50) // 40% 429 ratio
	current := makeReport(100, 90, 10, 50)  // 10% 429 ratio — dropped 75%
	regressions := Compare(baseline, current, Thresholds{P99PctIncrease: 10, R429PctDecrease: 10})
	found := false
	for _, r := range regressions {
		if r.Field == "rate_limit_ratio" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected rate_limit_ratio regression")
	}
}
