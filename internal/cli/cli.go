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

	"github.com/conantorreswf/limithit/internal/attacks/flood"
	"github.com/conantorreswf/limithit/internal/attacks/fuzz"
	"github.com/conantorreswf/limithit/internal/attacks/headerbomb"
	"github.com/conantorreswf/limithit/internal/attacks/slowloris"
	"github.com/conantorreswf/limithit/internal/attacks/spoof"
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
	case "flood":
		return runFlood(ctx, rest, stdout, stderr)
	case "slowloris":
		return runSlowloris(ctx, rest, stdout, stderr)
	case "spoof":
		return runSpoof(ctx, rest, stdout, stderr)
	case "fuzz":
		return runFuzz(ctx, rest, stdout, stderr)
	case "headerbomb":
		return runHeaderbomb(ctx, rest, stdout, stderr)
	case "-h", "--help", "help":
		printRoot(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", cmd)
		printRoot(stderr)
		return 2
	}
}

func printRoot(w io.Writer) {
	fmt.Fprintln(w, `limithit — HTTP attack-simulation toolkit

Usage:
  limithit <command> [flags] <url>

Commands:
  flood        High-throughput request flood (basic load/rate-limit probe)
  slowloris    Hold many connections open with slow header drip
  spoof        X-Forwarded-For rotation across an IP pool with pacing
  fuzz         Path enumeration from a wordlist (+ optional cache-bust)
  headerbomb   Oversized headers and progressively growing body

Run "limithit <command> -h" for command-specific flags.`)
}

func runFlood(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("flood", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		url         = fs.String("url", "", "target URL (or positional)")
		total       = fs.Int("total", 100, "total requests")
		concurrency = fs.Int("concurrency", 10, "worker count")
		method      = fs.String("method", "GET", "HTTP method")
		timeoutSec  = fs.Int("timeout", 10, "per-request timeout seconds")
		body        = fs.String("body", "", "request body")
		hdr         HeaderFlag
	)
	fs.Var(&hdr, "header", `custom header "Key: Value" (repeatable)`)
	urlArg, rest := extractURLArg(args)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if *url == "" {
		if urlArg != "" {
			*url = urlArg
		} else {
			*url = firstPositional(fs)
		}
	}
	if err := validateURL(*url); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 2
	}
	m, err := validateMethod(*method)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 2
	}
	if *total <= 0 || *concurrency <= 0 || *timeoutSec <= 0 {
		fmt.Fprintln(stderr, "error: total/concurrency/timeout must be > 0")
		return 2
	}
	rep := flood.Run(ctx, flood.Options{
		URL: *url, Method: m, Body: *body, Headers: hdr.Headers,
		Timeout: time.Duration(*timeoutSec) * time.Second,
		Total:   *total, Concurrency: *concurrency,
	})
	fmt.Fprint(stdout, rep.String())
	maybeInterrupted(ctx, stderr)
	return 0
}

func runSlowloris(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("slowloris", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		url         = fs.String("url", "", "target URL (or positional)")
		connections = fs.Int("connections", 200, "concurrent open connections")
		intervalSec = fs.Int("header-interval", 10, "seconds between drip headers")
		holdSec     = fs.Int("hold", 120, "total hold duration per connection (seconds)")
		dialSec     = fs.Int("dial-timeout", 5, "dial timeout seconds")
		insecure    = fs.Bool("insecure", false, "skip TLS verification")
	)
	urlArg, rest := extractURLArg(args)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if *url == "" {
		if urlArg != "" {
			*url = urlArg
		} else {
			*url = firstPositional(fs)
		}
	}
	if err := validateURL(*url); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 2
	}
	rep, err := slowloris.Run(ctx, slowloris.Options{
		URL: *url, Connections: *connections,
		HeaderInterval:  time.Duration(*intervalSec) * time.Second,
		Hold:            time.Duration(*holdSec) * time.Second,
		DialTimeout:     time.Duration(*dialSec) * time.Second,
		InsecureSkipTLS: *insecure,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	fmt.Fprint(stdout, rep.String())
	maybeInterrupted(ctx, stderr)
	return 0
}

func runSpoof(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("spoof", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		url         = fs.String("url", "", "target URL (or positional)")
		total       = fs.Int("total", 1000, "total requests")
		concurrency = fs.Int("concurrency", 20, "worker count")
		method      = fs.String("method", "GET", "HTTP method")
		body        = fs.String("body", "", "request body")
		timeoutSec  = fs.Int("timeout", 10, "per-request timeout seconds")
		ipPool      = fs.String("ip-pool", "", `IP pool spec ("10.0.0.0/24" | "file:ips.txt" | "1.2.3.4,5.6.7.8")`)
		pacing      = fs.String("pacing", "none", "pacer: uniform|poisson|zipf|none")
		minDelayMs  = fs.Int("min-delay-ms", 0, "min inter-request delay (ms, uniform/zipf)")
		maxDelayMs  = fs.Int("max-delay-ms", 50, "max inter-request delay (ms, uniform/zipf)")
		rps         = fs.Float64("rps", 50, "target rps (poisson)")
		xffHdr      = fs.String("xff-header", "X-Forwarded-For", "header used to inject spoofed IP")
		hdr         HeaderFlag
	)
	fs.Var(&hdr, "header", `extra header "Key: Value" (repeatable)`)
	urlArg, rest := extractURLArg(args)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if *url == "" {
		if urlArg != "" {
			*url = urlArg
		} else {
			*url = firstPositional(fs)
		}
	}
	if err := validateURL(*url); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 2
	}
	m, err := validateMethod(*method)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 2
	}
	rep, err := spoof.Run(ctx, spoof.Options{
		URL: *url, Method: m, Body: *body, Headers: hdr.Headers,
		Timeout: time.Duration(*timeoutSec) * time.Second,
		Total:   *total, Concurrency: *concurrency,
		IPPoolSpec: *ipPool, Pacing: *pacing,
		MinDelay: time.Duration(*minDelayMs) * time.Millisecond,
		MaxDelay: time.Duration(*maxDelayMs) * time.Millisecond,
		RPS:      *rps, XFFHeader: *xffHdr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	fmt.Fprint(stdout, rep.String())
	maybeInterrupted(ctx, stderr)
	return 0
}

func runFuzz(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fuzz", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		url         = fs.String("url", "", "base URL — scheme://host[:port] (or positional)")
		total       = fs.Int("total", 1000, "total requests")
		concurrency = fs.Int("concurrency", 20, "worker count")
		timeoutSec  = fs.Int("timeout", 10, "per-request timeout seconds")
		wordlist    = fs.String("wordlist", "", "path-list file (overrides embedded default)")
		cacheBust   = fs.Bool("cache-bust", false, "append random _cb=<hex> to each request")
		hdr         HeaderFlag
	)
	fs.Var(&hdr, "header", `extra header "Key: Value" (repeatable)`)
	urlArg, rest := extractURLArg(args)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if *url == "" {
		if urlArg != "" {
			*url = urlArg
		} else {
			*url = firstPositional(fs)
		}
	}
	if err := validateURL(*url); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 2
	}
	rep, err := fuzz.Run(ctx, fuzz.Options{
		BaseURL: *url, WordlistPth: *wordlist, CacheBust: *cacheBust,
		Headers: hdr.Headers, Timeout: time.Duration(*timeoutSec) * time.Second,
		Total: *total, Concurrency: *concurrency,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	fmt.Fprint(stdout, rep.String())
	maybeInterrupted(ctx, stderr)
	return 0
}

func runHeaderbomb(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("headerbomb", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		url         = fs.String("url", "", "target URL (or positional)")
		total       = fs.Int("total", 50, "total requests")
		concurrency = fs.Int("concurrency", 5, "worker count")
		method      = fs.String("method", "", "HTTP method (default POST if body>0 else GET)")
		timeoutSec  = fs.Int("timeout", 15, "per-request timeout seconds")
		headerCount = fs.Int("header-count", 500, "X-Junk headers per request")
		headerSize  = fs.Int("header-size", 1024, "bytes per junk header value")
		bodyStart   = fs.Int("body-start", 1024, "initial body size (bytes)")
		bodyMax     = fs.Int("body-max", 16<<20, "max body size (bytes)")
		bodyStep    = fs.Int("body-step", 0, "body growth step (0 = double each time)")
		hdr         HeaderFlag
	)
	fs.Var(&hdr, "header", `extra header "Key: Value" (repeatable)`)
	urlArg, rest := extractURLArg(args)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if *url == "" {
		if urlArg != "" {
			*url = urlArg
		} else {
			*url = firstPositional(fs)
		}
	}
	if err := validateURL(*url); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 2
	}
	var m string
	if *method != "" {
		var err error
		m, err = validateMethod(*method)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 2
		}
	}
	rep, err := headerbomb.Run(ctx, headerbomb.Options{
		URL: *url, Method: m, Headers: hdr.Headers,
		Timeout: time.Duration(*timeoutSec) * time.Second,
		Total:   *total, Concurrency: *concurrency,
		HeaderCount: *headerCount, HeaderSize: *headerSize,
		BodyStart: *bodyStart, BodyMax: *bodyMax, BodyStep: *bodyStep,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	fmt.Fprint(stdout, rep.String())
	maybeInterrupted(ctx, stderr)
	return 0
}

func maybeInterrupted(ctx context.Context, w io.Writer) {
	if ctx.Err() != nil {
		fmt.Fprintln(w, "\n(interrupted by signal — totals reflect requests sent before cancellation)")
	}
}
