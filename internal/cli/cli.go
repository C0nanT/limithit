package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/conantorreswf/limithit/internal/attacks"
	_ "github.com/conantorreswf/limithit/internal/attacks/all" // register all attack implementations
	"github.com/conantorreswf/limithit/internal/client"
	"github.com/conantorreswf/limithit/internal/config"
	"github.com/conantorreswf/limithit/internal/metrics"
	"github.com/conantorreswf/limithit/internal/report"
	"github.com/conantorreswf/limithit/internal/safety"
	"github.com/conantorreswf/limithit/internal/scenario"
	"github.com/conantorreswf/limithit/internal/version"
	"github.com/conantorreswf/limithit/internal/webui"
)

func Run(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) < 1 {
		return RunInteractive(ctx, stdout, stderr)
	}
	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "-h", "--help", "help":
		printRoot(stdout)
		return 0
	case "--version", "-v", "version":
		fmt.Fprintf(stdout, "limithit %s\n", version.Version)
		return 0
	case "init":
		return runInit(rest, stdout, stderr)
	case "run":
		return runScenario(ctx, rest, stdout, stderr)
	case "webui":
		return runWebUI(ctx, rest, stdout, stderr)
	default:
		a, ok := attacks.Lookup(cmd)
		if !ok {
			fmt.Fprintf(stderr, "unknown command: %s\n\n", cmd)
			printRoot(stderr)
			return 2
		}
		return runAttack(ctx, a, rest, stdout, stderr)
	}
}

func printRoot(w io.Writer) {
	fmt.Fprintf(w, "limithit %s — HTTP attack-simulation toolkit\n\n", version.Version)
	fmt.Fprintln(w, `Usage:
  limithit <command> [flags] <url>
  limithit run <scenario.yaml> [--continue-on-fail]
  limithit init [config.yaml]
  limithit webui [--port 9090]`)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Attack commands:")
	for _, a := range attacks.All() {
		fmt.Fprintf(w, "  %-12s %s\n", a.Name(), a.Synopsis())
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Scenario commands:")
	fmt.Fprintln(w, "  init          scaffold a starter limithit.yaml")
	fmt.Fprintln(w, "  run           execute a scenario file and print combined report")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Admin panel:")
	fmt.Fprintln(w, "  webui         start the browser-based admin panel (default :9090)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, `Run "limithit <command> -h" for command-specific flags.`)
}

func runInit(args []string, stdout, stderr io.Writer) int {
	path := "limithit.yaml"
	for _, a := range args {
		if a != "" && a[0] != '-' {
			path = a
			break
		}
	}
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(stderr, "error: %s already exists\n", path)
		return 2
	}
	if err := os.WriteFile(path, []byte(config.Scaffold()), 0644); err != nil {
		fmt.Fprintf(stderr, "error: write %s: %s\n", path, err)
		return 1
	}
	fmt.Fprintf(stdout, "created %s\n", path)
	return 0
}

func runScenario(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	continueOnFail := fs.Bool("continue-on-fail", false, "continue to next step on assertion failure")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "usage: limithit run <scenario.yaml> [--continue-on-fail]")
		return 2
	}
	cfg, err := config.Load(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 2
	}
	if err := scenario.Validate(cfg); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 2
	}
	return scenario.Run(ctx, cfg, stdout, stderr, *continueOnFail)
}

type outputConfig struct {
	fmt          string
	file         string
	expectStatus int
	compareTo    string
	cmpP99       float64
	cmp429       float64
	live         bool
	quiet        bool
	noColor      bool
	total        int
	auditLog     string
	safetyOpts   safety.Opts
}

// buildAttackBase parses flags, runs the safety guard, and returns an attacks.Base.
// ProgressCh is not set here — the caller sets it based on outCfg.live.
func buildAttackBase(a attacks.Attack, args []string, stderr io.Writer) (attacks.Base, *outputConfig, int) {
	fs := flag.NewFlagSet(a.Name(), flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		urlFlag      = fs.String("url", "", "target URL (or positional)")
		total        = fs.Int("total", 100, "total requests")
		concurrency  = fs.Int("concurrency", 10, "worker count")
		timeoutSec   = fs.Int("timeout", 10, "per-request timeout seconds")
		keepalive    = fs.Bool("keepalive", true, "enable HTTP keep-alive")
		expectStatus = fs.Int("expect-status", 0, "assert HTTP status code appears ≥1 time; exit 1 if not")
		rampStart    = fs.Float64("ramp-start", 0, "start RPS for linear ramp (0 = disabled)")
		rampDur      = fs.String("ramp-duration", "30s", "ramp duration")
		outputFmt    = fs.String("output", "table", "output format: table|json|csv")
		outputFile   = fs.String("output-file", "", "write output to this file path")
		compareTo    = fs.String("compare", "", "path to baseline JSON; exits non-zero on regression")
		cmpP99       = fs.Float64("compare-p99-threshold", 10.0, "p99 latency % increase regression threshold")
		cmp429       = fs.Float64("compare-429-threshold", 10.0, "429 ratio % decrease regression threshold")
		live         = fs.Bool("live", false, "print live progress to stderr during run")
		quiet        = fs.Bool("quiet", false, "suppress informational output; report goes to stdout")
		noColor      = fs.Bool("no-color", false, "disable ANSI colour in output")
		iUnderstand  = fs.Bool("i-understand", false, "affirm authorisation for destructive/amplifying attacks")
		maxRPS       = fs.Float64("max-rps", 0, "global RPS cap via poisson pacer (0 = uncapped)")
		allowTarget  = fs.String("allow-target", "", `suppress non-loopback warning (host or "*")`)
		auditLog     = fs.String("audit-log", "", "path to JSONL audit log (appended per run)")
		hdr          HeaderFlag
	)
	fs.Var(&hdr, "header", `custom header "Key: Value" (repeatable)`)

	a.Flags(fs)

	urlArg, rest := extractURLArg(args)
	if err := fs.Parse(rest); err != nil {
		return attacks.Base{}, nil, 2
	}

	if *urlFlag == "" {
		if urlArg != "" {
			*urlFlag = urlArg
		} else {
			*urlFlag = firstPositional(fs)
		}
	}
	if err := validateURL(*urlFlag); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return attacks.Base{}, nil, 2
	}
	if err := a.Validate(); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return attacks.Base{}, nil, 2
	}

	timeout := time.Duration(*timeoutSec) * time.Second

	var pacer metrics.Pacer
	if *rampStart > 0 {
		rd, err := time.ParseDuration(*rampDur)
		if err != nil {
			fmt.Fprintf(stderr, "error: invalid --ramp-duration: %s\n", err)
			return attacks.Base{}, nil, 2
		}
		p, err := metrics.NewRampPacer(*rampStart, float64(*concurrency), rd)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return attacks.Base{}, nil, 2
		}
		pacer = p
	} else if *maxRPS > 0 {
		p, err := metrics.NewPacer("poisson", 0, 0, *maxRPS)
		if err != nil {
			fmt.Fprintf(stderr, "error: invalid --max-rps: %s\n", err)
			return attacks.Base{}, nil, 2
		}
		pacer = p
	}

	var allowTargets []string
	if *allowTarget != "" {
		allowTargets = strings.Split(*allowTarget, ",")
	}

	sopts := safety.Opts{
		IUnderstand:  *iUnderstand,
		AllowTargets: allowTargets,
		MaxRPS:       *maxRPS,
	}
	if err := safety.Confirm(a.Name(), *urlFlag, sopts, stderr); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return attacks.Base{}, nil, 1
	}

	base := attacks.Base{
		URL: *urlFlag,
		Client: client.New(client.Config{
			Timeout:           timeout,
			DisableKeepAlives: !*keepalive,
		}, *concurrency),
		Common: attacks.CommonOpts{
			Total:       *total,
			Concurrency: *concurrency,
			Timeout:     timeout,
			Headers:     hdr.Headers,
			Pacer:       pacer,
		},
	}

	outCfg := &outputConfig{
		fmt:          *outputFmt,
		file:         *outputFile,
		expectStatus: *expectStatus,
		compareTo:    *compareTo,
		cmpP99:       *cmpP99,
		cmp429:       *cmp429,
		live:         *live,
		quiet:        *quiet,
		noColor:      *noColor,
		total:        *total,
		auditLog:     *auditLog,
		safetyOpts:   sopts,
	}

	return base, outCfg, 0
}

func runAttack(ctx context.Context, a attacks.Attack, args []string, stdout, stderr io.Writer) int {
	base, outCfg, code := buildAttackBase(a, args, stderr)
	if code != 0 {
		return code
	}

	var liveWG sync.WaitGroup
	if outCfg.live && !outCfg.quiet {
		progressCh := make(chan metrics.Progress, 4)
		base.ProgressCh = progressCh
		liveWG.Add(1)
		go func() {
			defer liveWG.Done()
			printLiveProgress(progressCh, outCfg.total, stderr)
		}()
	}

	start := time.Now()
	rep, err := a.Run(ctx, base)

	if base.ProgressCh != nil {
		close(base.ProgressCh)
		liveWG.Wait()
	}

	writeAuditLog(outCfg.auditLog, a.Name(), base.URL, outCfg, start, err)

	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if !outCfg.quiet {
		maybeInterrupted(ctx, stderr)
	}

	outW := stdout
	if outCfg.file != "" {
		f, ferr := os.Create(outCfg.file)
		if ferr != nil {
			fmt.Fprintf(stderr, "error: open output file: %s\n", ferr)
			return 1
		}
		defer f.Close()
		outW = f
	}

	if r, ok := rep.(*metrics.Report); ok {
		switch outCfg.fmt {
		case "json":
			if err := report.JSON(outW, r); err != nil {
				fmt.Fprintf(stderr, "error: json: %s\n", err)
				return 1
			}
		case "csv":
			if err := report.CSV(outW, r); err != nil {
				fmt.Fprintf(stderr, "error: csv: %s\n", err)
				return 1
			}
		default:
			report.Table(outW, r)
		}
	} else {
		fmt.Fprint(outW, rep.String())
	}

	if outCfg.expectStatus != 0 {
		if r, ok := rep.(*metrics.Report); ok {
			if r.StatusCounts[outCfg.expectStatus] == 0 {
				fmt.Fprintf(stderr, "assertion failed: expected HTTP %d but it was not observed\n", outCfg.expectStatus)
				return 1
			}
		}
	}

	if outCfg.compareTo != "" {
		r, ok := rep.(*metrics.Report)
		if !ok {
			fmt.Fprintln(stderr, "warning: --compare not supported for this attack type")
			return 0
		}
		baseline, berr := report.LoadBaseline(outCfg.compareTo)
		if berr != nil {
			fmt.Fprintf(stderr, "error: load baseline: %s\n", berr)
			return 1
		}
		regressions := report.Compare(baseline, r, report.Thresholds{
			P99PctIncrease:  outCfg.cmpP99,
			R429PctDecrease: outCfg.cmp429,
		})
		if len(regressions) > 0 {
			fmt.Fprintln(stderr, "\nregressions detected:")
			for _, reg := range regressions {
				fmt.Fprintf(stderr, "  FAIL  %s\n", reg.Message)
			}
			return 1
		}
		fmt.Fprintln(outW, "compare: no regressions detected")
	}

	return 0
}

type auditRecord struct {
	Timestamp string  `json:"timestamp"`
	Version   string  `json:"version"`
	Attack    string  `json:"attack"`
	Target    string  `json:"target"`
	Total     int     `json:"total"`
	MaxRPS    float64 `json:"max_rps,omitempty"`
	Error     string  `json:"error,omitempty"`
}

func writeAuditLog(path, attackName, target string, cfg *outputConfig, start time.Time, runErr error) {
	if path == "" {
		return
	}
	rec := auditRecord{
		Timestamp: start.UTC().Format(time.RFC3339),
		Version:   version.Version,
		Attack:    attackName,
		Target:    target,
		Total:     cfg.total,
		MaxRPS:    cfg.safetyOpts.MaxRPS,
	}
	if runErr != nil {
		rec.Error = runErr.Error()
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(rec)
}

func printLiveProgress(ch <-chan metrics.Progress, _ int, w io.Writer) {
	for p := range ch {
		pct := 0.0
		if p.Total > 0 {
			pct = float64(p.Sent) / float64(p.Total) * 100
		}
		fmt.Fprintf(w, "\r  %d/%d (%.0f%%)  %.1f rps  2xx:%d  429:%d  err:%d  %s   ",
			p.Sent, p.Total, pct, p.RPS, p.Success, p.RateLimited, p.OtherErr,
			p.Elapsed.Round(time.Second))
	}
	fmt.Fprintln(w)
}

func maybeInterrupted(ctx context.Context, w io.Writer) {
	if ctx.Err() != nil {
		fmt.Fprintln(w, "\n(interrupted by signal — totals reflect requests sent before cancellation)")
	}
}

func runWebUI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("webui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.String("port", "9090", "port to listen on")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	addr := "127.0.0.1:" + *port
	fmt.Fprintf(stdout, "limithit admin panel → http://%s\n", addr)
	fmt.Fprintln(stdout, "Press Ctrl+C to stop.")
	if err := webui.Serve(ctx, addr); err != nil {
		fmt.Fprintf(stderr, "webui: %s\n", err)
		return 1
	}
	return 0
}
