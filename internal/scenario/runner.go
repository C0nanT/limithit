package scenario

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"sort"
	"time"

	"github.com/conantorreswf/limithit/internal/attacks"
	_ "github.com/conantorreswf/limithit/internal/attacks/all" // register all attacks
	"github.com/conantorreswf/limithit/internal/client"
	"github.com/conantorreswf/limithit/internal/config"
	"github.com/conantorreswf/limithit/internal/metrics"
	"github.com/conantorreswf/limithit/internal/report"
)

// Result holds the outcome of a single scenario step.
type Result struct {
	StepNum int
	Attack  string
	Report  attacks.Report
	Err     error
	Failed  bool // expect-status assertion failed
}

// Validate checks target URL, attack names, and flags against the registry before sending traffic.
func Validate(cfg *config.Config) error {
	if err := validateURL(cfg.Target); err != nil {
		return fmt.Errorf("target: %w", err)
	}
	for i, step := range cfg.Scenario {
		a, ok := attacks.Lookup(step.Attack)
		if !ok {
			return fmt.Errorf("step %d: unknown attack %q", i+1, step.Attack)
		}
		fs := flag.NewFlagSet(step.Attack, flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		registerCommonFlags(fs)
		a.Flags(fs)
		if err := fs.Parse(stepArgs(cfg, step)); err != nil {
			return fmt.Errorf("step %d (%s): %w", i+1, step.Attack, err)
		}
	}
	return nil
}

// Run executes all scenario steps sequentially and prints a combined report.
func Run(ctx context.Context, cfg *config.Config, stdout, stderr io.Writer, continueOnFail bool) int {
	results := make([]Result, 0, len(cfg.Scenario))
	exitCode := 0

	for i, step := range cfg.Scenario {
		fmt.Fprintf(stdout, "\n=== step %d/%d: %s ===\n", i+1, len(cfg.Scenario), step.Attack)

		a, _ := attacks.Lookup(step.Attack)
		r := execStep(ctx, cfg, step, a, stderr)
		r.StepNum = i + 1
		results = append(results, r)

		if r.Err != nil || r.Failed {
			exitCode = 1
			if !continueOnFail {
				fmt.Fprintf(stderr, "step %d failed — stopping (use --continue-on-fail to proceed)\n", i+1)
				break
			}
		}
	}

	printSummary(stdout, results, outputFmt(cfg))
	return exitCode
}

type stepExecOpts struct {
	total        int
	concurrency  int
	timeoutSec   int
	keepalive    bool
	expectStatus int
	outputFmt    string
}

func execStep(ctx context.Context, cfg *config.Config, step config.Step, a attacks.Attack, stderr io.Writer) Result {
	fs := flag.NewFlagSet(step.Attack, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts stepExecOpts
	urlFlag := fs.String("url", cfg.Target, "")
	fs.IntVar(&opts.total, "total", 100, "")
	fs.IntVar(&opts.concurrency, "concurrency", 10, "")
	fs.IntVar(&opts.timeoutSec, "timeout", 10, "")
	fs.BoolVar(&opts.keepalive, "keepalive", true, "")
	fs.IntVar(&opts.expectStatus, "expect-status", 0, "")
	fs.StringVar(&opts.outputFmt, "output", "table", "")
	a.Flags(fs)

	if err := fs.Parse(stepArgs(cfg, step)); err != nil {
		return Result{Attack: step.Attack, Err: fmt.Errorf("parse flags: %w", err)}
	}
	if err := a.Validate(); err != nil {
		return Result{Attack: step.Attack, Err: fmt.Errorf("validate: %w", err)}
	}

	timeout := time.Duration(opts.timeoutSec) * time.Second
	base := attacks.Base{
		URL: *urlFlag,
		Client: client.New(client.Config{
			Timeout:           timeout,
			DisableKeepAlives: !opts.keepalive,
		}, opts.concurrency),
		Common: attacks.CommonOpts{
			Total:       opts.total,
			Concurrency: opts.concurrency,
			Timeout:     timeout,
		},
	}

	rep, err := a.Run(ctx, base)
	if err != nil {
		return Result{Attack: step.Attack, Err: err}
	}

	failed := false
	if opts.expectStatus != 0 {
		if r, ok := rep.(*metrics.Report); ok {
			if r.StatusCounts[opts.expectStatus] == 0 {
				failed = true
				fmt.Fprintf(stderr, "assertion failed: step expected HTTP %d but it was not observed\n", opts.expectStatus)
			}
		}
	}

	return Result{Attack: step.Attack, Report: rep, Failed: failed}
}

// stepArgs builds []string flag args from config defaults + step flags.
// Step flags override defaults when both define the same key.
func stepArgs(cfg *config.Config, step config.Step) []string {
	merged := make(map[string]string)
	cfg.Defaults.Each(func(k, v string) { merged[k] = v })
	for k, v := range step.Flags {
		merged[k] = v
	}

	args := make([]string, 0, len(merged)*2+1)
	// Sort for deterministic flag order (aids testing + readability in errors)
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--"+k, merged[k])
	}
	args = append(args, cfg.Target)
	return args
}

func validateURL(raw string) error {
	if raw == "" {
		return errors.New("url is required")
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("url scheme must be http or https")
	}
	return nil
}

func registerCommonFlags(fs *flag.FlagSet) {
	fs.String("url", "", "")
	fs.Int("total", 100, "")
	fs.Int("concurrency", 10, "")
	fs.Int("timeout", 10, "")
	fs.Bool("keepalive", true, "")
	fs.Int("expect-status", 0, "")
	fs.String("output", "table", "")
}

func outputFmt(cfg *config.Config) string {
	if v, ok := cfg.Defaults.Get("output"); ok {
		return v
	}
	return "table"
}

func printSummary(w io.Writer, results []Result, fmtStr string) {
	fmt.Fprintln(w, "\n=== scenario summary ===")

	if fmtStr == "json" {
		printSummaryJSON(w, results)
		return
	}

	printSummaryTable(w, results)
}

func printSummaryTable(w io.Writer, results []Result) {
	pass, fail := 0, 0
	for _, r := range results {
		status := "PASS"
		if r.Err != nil || r.Failed {
			status = "FAIL"
			fail++
		} else {
			pass++
		}

		line := fmt.Sprintf("  [%s] step %d: %s", status, r.StepNum, r.Attack)
		if r.Err != nil {
			line += fmt.Sprintf(" — error: %v", r.Err)
		} else if r.Report != nil {
			if rep, ok := r.Report.(*metrics.Report); ok {
				line += fmt.Sprintf(" — sent=%d ok=%d 429=%d p99=%s",
					rep.Sent, rep.Success, rep.TooMany, rep.Latency.P99)
			}
		}
		fmt.Fprintln(w, line)
	}
	fmt.Fprintf(w, "\n  %d passed, %d failed\n", pass, fail)
}

type jsonResult struct {
	StepNum int    `json:"step"`
	Attack  string `json:"attack"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

func printSummaryJSON(w io.Writer, results []Result) {
	enc := newJSONEncoder(w)
	out := make([]jsonResult, 0, len(results))
	for _, r := range results {
		jr := jsonResult{StepNum: r.StepNum, Attack: r.Attack, Status: "pass"}
		if r.Err != nil || r.Failed {
			jr.Status = "fail"
			if r.Err != nil {
				jr.Error = r.Err.Error()
			}
		}
		out = append(out, jr)
	}
	enc(out)
}

func newJSONEncoder(w io.Writer) func(interface{}) {
	return func(v interface{}) {
		_ = report.JSONAny(w, v)
	}
}
