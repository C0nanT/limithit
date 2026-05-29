package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/conantorreswf/limithit/internal/attacks"
	_ "github.com/conantorreswf/limithit/internal/attacks/all"
	"github.com/conantorreswf/limithit/internal/client"
	"github.com/conantorreswf/limithit/internal/metrics"
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
	fmt.Fprintln(w, `limithit — HTTP attack-simulation toolkit

Usage:
  limithit <command> [flags] <url>`)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, a := range attacks.All() {
		fmt.Fprintf(w, "  %-12s %s\n", a.Name(), a.Synopsis())
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, `Run "limithit <command> -h" for command-specific flags.`)
}

func runAttack(ctx context.Context, a attacks.Attack, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(a.Name(), flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		urlFlag      = fs.String("url", "", "target URL (or positional)")
		total        = fs.Int("total", 100, "total requests")
		concurrency  = fs.Int("concurrency", 10, "worker count")
		timeoutSec   = fs.Int("timeout", 10, "per-request timeout seconds")
		keepalive    = fs.Bool("keepalive", true, "enable HTTP keep-alive (false = new TCP/TLS per request)")
		expectStatus = fs.Int("expect-status", 0, "assert this HTTP status code appears ≥1 time; exit 1 if not")
		rampStart    = fs.Float64("ramp-start", 0, "start RPS for linear ramp (0 = disabled)")
		rampDur      = fs.String("ramp-duration", "30s", "duration to ramp from --ramp-start to full rate")
		hdr          HeaderFlag
	)
	fs.Var(&hdr, "header", `custom header "Key: Value" (repeatable)`)

	a.Flags(fs)

	urlArg, rest := extractURLArg(args)
	if err := fs.Parse(rest); err != nil {
		return 2
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
		return 2
	}
	if err := a.Validate(); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 2
	}

	timeout := time.Duration(*timeoutSec) * time.Second

	var pacer metrics.Pacer
	if *rampStart > 0 {
		rd, err := time.ParseDuration(*rampDur)
		if err != nil {
			fmt.Fprintf(stderr, "error: invalid --ramp-duration: %s\n", err)
			return 2
		}
		p, err := metrics.NewRampPacer(*rampStart, float64(*concurrency), rd)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 2
		}
		pacer = p
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

	rep, err := a.Run(ctx, base)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	fmt.Fprint(stdout, rep.String())
	maybeInterrupted(ctx, stderr)

	if *expectStatus != 0 {
		if r, ok := rep.(*metrics.Report); ok {
			if r.StatusCounts[*expectStatus] == 0 {
				fmt.Fprintf(stderr, "assertion failed: expected HTTP %d but it was not observed\n", *expectStatus)
				return 1
			}
		}
	}
	return 0
}

func maybeInterrupted(ctx context.Context, w io.Writer) {
	if ctx.Err() != nil {
		fmt.Fprintln(w, "\n(interrupted by signal — totals reflect requests sent before cancellation)")
	}
}
