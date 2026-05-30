package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/charmbracelet/huh"
	"github.com/conantorreswf/limithit/internal/attacks"
)

func RunInteractive(ctx context.Context, stdout, stderr io.Writer) int {
	all := attacks.All()
	opts := make([]huh.Option[string], len(all))
	for i, a := range all {
		opts[i] = huh.NewOption(fmt.Sprintf("%-12s %s", a.Name(), a.Synopsis()), a.Name())
	}

	var attack string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("limithit — select attack").
				Options(opts...).
				Value(&attack),
		),
	).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	switch attack {
	case "flood":
		return interactiveFlood(ctx, stdout, stderr)
	case "slowloris":
		return interactiveSlowloris(ctx, stdout, stderr)
	case "spoof":
		return interactiveSpoof(ctx, stdout, stderr)
	case "fuzz":
		return interactiveFuzz(ctx, stdout, stderr)
	case "headerbomb":
		return interactiveHeaderbomb(ctx, stdout, stderr)
	case "h2flood":
		return interactiveH2Flood(ctx, stdout, stderr)
	case "wsflood":
		return interactiveWSFlood(ctx, stdout, stderr)
	case "gzipbomb":
		return interactiveGzipBomb(ctx, stdout, stderr)
	case "replay":
		return interactiveReplay(ctx, stdout, stderr)
	case "methodspray":
		return interactiveMethodSpray(ctx, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "error: no interactive form for attack %q — use CLI flags instead\n", attack)
		return 2
	}
}

// sharedOpts holds common engine/output options appended to every interactive form.
type sharedOpts struct {
	outputFmt    string
	outputFile   string
	expectStatus string
	keepalive    bool
}

func defaultSharedOpts() sharedOpts {
	return sharedOpts{
		outputFmt:    "table",
		expectStatus: "0",
		keepalive:    true,
	}
}

func sharedOptsGroup(o *sharedOpts) *huh.Group {
	return huh.NewGroup(
		huh.NewSelect[string]().
			Title("Output format").
			Options(
				huh.NewOption("table (human-readable)", "table"),
				huh.NewOption("json (machine-readable)", "json"),
				huh.NewOption("csv (spreadsheet)", "csv"),
			).
			Value(&o.outputFmt),
		huh.NewInput().
			Title("Output file (empty = stdout)").
			Value(&o.outputFile),
		huh.NewInput().
			Title("Assert HTTP status code (0 = disabled)").
			Description("Exit non-zero if this status code is never observed").
			Value(&o.expectStatus).
			Validate(validateNonNegativeInt),
		huh.NewConfirm().
			Title("Enable HTTP keep-alive").
			Description("Disable to force a new TCP/TLS handshake per request").
			Value(&o.keepalive),
	)
}

func (o sharedOpts) args() []string {
	var args []string
	if o.outputFmt != "" && o.outputFmt != "table" {
		args = append(args, "-output", o.outputFmt)
	}
	if o.outputFile != "" {
		args = append(args, "-output-file", o.outputFile)
	}
	if o.expectStatus != "0" && o.expectStatus != "" {
		args = append(args, "-expect-status", o.expectStatus)
	}
	if !o.keepalive {
		args = append(args, "-keepalive=false")
	}
	return args
}

func interactiveFlood(ctx context.Context, stdout, stderr io.Writer) int {
	var (
		url         = ""
		total       = "100"
		concurrency = "10"
		method      = "GET"
		timeoutSec  = "10"
		body        = ""
	)
	shared := defaultSharedOpts()

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Target URL").Placeholder("https://example.com").Value(&url).Validate(validateURL),
			huh.NewInput().Title("Total requests").Value(&total).Validate(validatePositiveInt),
			huh.NewInput().Title("Concurrency (workers)").Value(&concurrency).Validate(validatePositiveInt),
			methodSelect(&method),
			huh.NewInput().Title("Timeout (s)").Value(&timeoutSec).Validate(validatePositiveInt),
			huh.NewInput().Title("Request body (optional)").Value(&body),
		),
		sharedOptsGroup(&shared),
	).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	args := []string{url, "-total", total, "-concurrency", concurrency, "-method", method, "-timeout", timeoutSec}
	if body != "" {
		args = append(args, "-body", body)
	}
	args = append(args, shared.args()...)
	return dispatchInteractive(ctx, "flood", args, stdout, stderr)
}

func interactiveSlowloris(ctx context.Context, stdout, stderr io.Writer) int {
	var (
		url         = ""
		connections = "200"
		intervalSec = "10"
		holdSec     = "120"
		dialSec     = "5"
		insecure    = false
	)
	shared := defaultSharedOpts()

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Target URL").Placeholder("https://example.com").Value(&url).Validate(validateURL),
			huh.NewInput().Title("Connections").Value(&connections).Validate(validatePositiveInt),
			huh.NewInput().Title("Header interval (s)").Value(&intervalSec).Validate(validatePositiveInt),
			huh.NewInput().Title("Hold duration (s)").Value(&holdSec).Validate(validatePositiveInt),
			huh.NewInput().Title("Dial timeout (s)").Value(&dialSec).Validate(validatePositiveInt),
			huh.NewConfirm().Title("Skip TLS verification").Value(&insecure),
		),
		sharedOptsGroup(&shared),
	).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	args := []string{
		url,
		"-connections", connections,
		"-header-interval", intervalSec,
		"-hold", holdSec,
		"-dial-timeout", dialSec,
	}
	if insecure {
		args = append(args, "-insecure")
	}
	args = append(args, shared.args()...)
	return dispatchInteractive(ctx, "slowloris", args, stdout, stderr)
}

func interactiveSpoof(ctx context.Context, stdout, stderr io.Writer) int {
	var (
		url         = ""
		total       = "1000"
		concurrency = "20"
		method      = "GET"
		timeoutSec  = "10"
		ipPool      = ""
		xffHeader   = "X-Forwarded-For"
		pacing      = "none"
		rps         = "50"
		minDelayMs  = "0"
		maxDelayMs  = "50"
	)
	shared := defaultSharedOpts()

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Target URL").Placeholder("https://example.com").Value(&url).Validate(validateURL),
			huh.NewInput().Title("Total requests").Value(&total).Validate(validatePositiveInt),
			huh.NewInput().Title("Concurrency (workers)").Value(&concurrency).Validate(validatePositiveInt),
			methodSelect(&method),
			huh.NewInput().Title("Timeout (s)").Value(&timeoutSec).Validate(validatePositiveInt),
		),
		huh.NewGroup(
			huh.NewInput().Title("IP pool (CIDR / file:path / comma list)").Value(&ipPool),
			huh.NewInput().
				Title("XFF header name").
				Description("Header used to inject spoofed IP").
				Value(&xffHeader),
			huh.NewSelect[string]().Title("Pacing").Options(
				huh.NewOption("none", "none"),
				huh.NewOption("uniform", "uniform"),
				huh.NewOption("poisson", "poisson"),
				huh.NewOption("zipf", "zipf"),
			).Value(&pacing),
			huh.NewInput().Title("Target RPS (poisson)").Value(&rps).Validate(validatePositiveFloat),
			huh.NewInput().Title("Min delay ms (uniform/zipf)").Value(&minDelayMs).Validate(validateNonNegativeInt),
			huh.NewInput().Title("Max delay ms (uniform/zipf)").Value(&maxDelayMs).Validate(validatePositiveInt),
		),
		sharedOptsGroup(&shared),
	).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	args := []string{
		url, "-total", total, "-concurrency", concurrency,
		"-method", method, "-timeout", timeoutSec,
		"-pacing", pacing, "-rps", rps,
		"-min-delay-ms", minDelayMs, "-max-delay-ms", maxDelayMs,
	}
	if ipPool != "" {
		args = append(args, "-ip-pool", ipPool)
	}
	if xffHeader != "" && xffHeader != "X-Forwarded-For" {
		args = append(args, "-xff-header", xffHeader)
	}
	args = append(args, shared.args()...)
	return dispatchInteractive(ctx, "spoof", args, stdout, stderr)
}

func interactiveFuzz(ctx context.Context, stdout, stderr io.Writer) int {
	var (
		url         = ""
		total       = "1000"
		concurrency = "20"
		timeoutSec  = "10"
		wordlist    = ""
		cacheBust   = false
	)
	shared := defaultSharedOpts()

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Base URL").Placeholder("https://example.com").Value(&url).Validate(validateURL),
			huh.NewInput().Title("Total requests").Value(&total).Validate(validatePositiveInt),
			huh.NewInput().Title("Concurrency (workers)").Value(&concurrency).Validate(validatePositiveInt),
			huh.NewInput().Title("Timeout (s)").Value(&timeoutSec).Validate(validatePositiveInt),
			huh.NewInput().Title("Wordlist path (empty = use built-in)").Value(&wordlist),
			huh.NewConfirm().Title("Cache bust (append _cb=<hex>)").Value(&cacheBust),
		),
		sharedOptsGroup(&shared),
	).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	args := []string{url, "-total", total, "-concurrency", concurrency, "-timeout", timeoutSec}
	if wordlist != "" {
		args = append(args, "-wordlist", wordlist)
	}
	if cacheBust {
		args = append(args, "-cache-bust")
	}
	args = append(args, shared.args()...)
	return dispatchInteractive(ctx, "fuzz", args, stdout, stderr)
}

func interactiveHeaderbomb(ctx context.Context, stdout, stderr io.Writer) int {
	var (
		url         = ""
		total       = "50"
		concurrency = "5"
		method      = ""
		timeoutSec  = "15"
		headerCount = "500"
		headerSize  = "1024"
		bodyStart   = "1024"
		bodyMax     = "16777216"
		bodyStep    = "0"
	)
	shared := defaultSharedOpts()

	methodOpts := append([]huh.Option[string]{
		huh.NewOption("auto (POST if body > 0, else GET)", ""),
	}, httpMethodOptions()...)

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Target URL").Placeholder("https://example.com").Value(&url).Validate(validateURL),
			huh.NewInput().Title("Total requests").Value(&total).Validate(validatePositiveInt),
			huh.NewInput().Title("Concurrency (workers)").Value(&concurrency).Validate(validatePositiveInt),
			huh.NewSelect[string]().Title("HTTP method").Options(methodOpts...).Value(&method),
			huh.NewInput().Title("Timeout (s)").Value(&timeoutSec).Validate(validatePositiveInt),
		),
		huh.NewGroup(
			huh.NewInput().Title("Junk header count").Value(&headerCount).Validate(validatePositiveInt),
			huh.NewInput().Title("Bytes per junk header").Value(&headerSize).Validate(validatePositiveInt),
			huh.NewInput().Title("Initial body size (bytes)").Value(&bodyStart).Validate(validatePositiveInt),
			huh.NewInput().Title("Max body size (bytes)").Value(&bodyMax).Validate(validatePositiveInt),
			huh.NewInput().Title("Body growth step (0 = double each time)").Value(&bodyStep).Validate(validateNonNegativeInt),
		),
		sharedOptsGroup(&shared),
	).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	args := []string{
		url, "-total", total, "-concurrency", concurrency,
		"-timeout", timeoutSec,
		"-header-count", headerCount, "-header-size", headerSize,
		"-body-start", bodyStart, "-body-max", bodyMax, "-body-step", bodyStep,
	}
	if method != "" {
		args = append(args, "-method", method)
	}
	args = append(args, shared.args()...)
	return dispatchInteractive(ctx, "headerbomb", args, stdout, stderr)
}

func interactiveH2Flood(ctx context.Context, stdout, stderr io.Writer) int {
	var (
		url         = ""
		connections = "1"
		streams     = "100"
		total       = "100"
		concurrency = "10"
		method      = "GET"
		insecure    = false
	)
	shared := defaultSharedOpts()

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Target URL (HTTPS recommended for HTTP/2)").
				Placeholder("https://example.com").
				Value(&url).
				Validate(validateURL),
			huh.NewInput().
				Title("HTTP/2 connections").
				Description("Number of long-lived TCP connections to open").
				Value(&connections).
				Validate(validatePositiveInt),
			huh.NewInput().
				Title("Concurrent streams per connection").
				Description("Exploits MaxConcurrentStreams gaps — try 100–1000").
				Value(&streams).
				Validate(validatePositiveInt),
			huh.NewInput().Title("Total requests").Value(&total).Validate(validatePositiveInt),
			huh.NewInput().Title("Concurrency (workers)").Value(&concurrency).Validate(validatePositiveInt),
			methodSelect(&method),
			huh.NewConfirm().Title("Skip TLS verification").Value(&insecure),
		),
		sharedOptsGroup(&shared),
	).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	args := []string{
		url,
		"-connections", connections,
		"-streams", streams,
		"-total", total,
		"-concurrency", concurrency,
		"-method", method,
	}
	if insecure {
		args = append(args, "-insecure")
	}
	args = append(args, shared.args()...)
	return dispatchInteractive(ctx, "h2flood", args, stdout, stderr)
}

func interactiveWSFlood(ctx context.Context, stdout, stderr io.Writer) int {
	var (
		url         = ""
		connections = "200"
		holdSec     = "60"
		msgRate     = "0"
		dialTimeout = "5"
		insecure    = false
	)
	shared := defaultSharedOpts()

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Target URL").
				Description("Use http:// or ws:// for plain WebSocket; https:// or wss:// for TLS").
				Placeholder("ws://localhost:8080/ws").
				Value(&url).
				Validate(validateURL),
			huh.NewInput().
				Title("WebSocket connections to open and hold").
				Value(&connections).
				Validate(validatePositiveInt),
			huh.NewInput().
				Title("Hold duration (seconds)").
				Description("How long to keep each connection open").
				Value(&holdSec).
				Validate(validatePositiveInt),
			huh.NewInput().
				Title("Ping messages per second (0 = silent hold)").
				Value(&msgRate).
				Validate(validateNonNegativeInt),
			huh.NewInput().Title("Dial timeout (s)").Value(&dialTimeout).Validate(validatePositiveInt),
			huh.NewConfirm().Title("Skip TLS verification").Value(&insecure),
		),
		sharedOptsGroup(&shared),
	).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	args := []string{
		url,
		"-connections", connections,
		"-hold", holdSec,
		"-message-rate", msgRate,
		"-dial-timeout", dialTimeout,
	}
	if insecure {
		args = append(args, "-insecure")
	}
	args = append(args, shared.args()...)
	return dispatchInteractive(ctx, "wsflood", args, stdout, stderr)
}

func interactiveGzipBomb(ctx context.Context, stdout, stderr io.Writer) int {
	var (
		url         = ""
		expandedMB  = "10"
		total       = "10"
		concurrency = "5"
		method      = "POST"
		confirmed   = false
	)
	shared := defaultSharedOpts()

	// Safety gate shown first as its own page.
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("WARNING: Decompression amplification attack").
				Description("This attack sends gzip-compressed bodies that expand large server-side.\n"+
					"A vulnerable server that decompresses the body may crash or OOM.\n"+
					"Only use against systems you own or have explicit written authorisation to test."),
			huh.NewConfirm().
				Title("I understand the risk and have authorisation to run this test").
				Value(&confirmed),
		),
	).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if !confirmed {
		fmt.Fprintln(stderr, "aborted: safety acknowledgement required")
		return 1
	}

	err = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Target URL").Placeholder("https://example.com").Value(&url).Validate(validateURL),
			huh.NewInput().
				Title("Expanded body size (MB)").
				Description("Uncompressed size the server must decompress per request").
				Value(&expandedMB).
				Validate(validatePositiveInt),
			huh.NewInput().Title("Total requests").Value(&total).Validate(validatePositiveInt),
			huh.NewInput().Title("Concurrency (workers)").Value(&concurrency).Validate(validatePositiveInt),
			methodSelect(&method),
		),
		sharedOptsGroup(&shared),
	).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	args := []string{
		url,
		"-expanded-mb", expandedMB,
		"-total", total,
		"-concurrency", concurrency,
		"-method", method,
		"-i-understand",
	}
	args = append(args, shared.args()...)
	return dispatchInteractive(ctx, "gzipbomb", args, stdout, stderr)
}

func interactiveReplay(ctx context.Context, stdout, stderr io.Writer) int {
	var (
		file        = ""
		targetURL   = ""
		total       = "0"
		concurrency = "10"
		loop        = false
	)
	shared := defaultSharedOpts()

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Request file").
				Description("HAR file or newline-delimited \"METHOD URL\" file").
				Placeholder("/path/to/requests.har").
				Value(&file),
			huh.NewInput().
				Title("Base URL (for report label)").
				Description("Individual request URLs come from the file; this is used as the report target label").
				Placeholder("https://example.com").
				Value(&targetURL).
				Validate(validateURL),
			huh.NewInput().
				Title("Total requests (0 = use file length)").
				Value(&total).
				Validate(validateNonNegativeInt),
			huh.NewInput().Title("Concurrency (workers)").Value(&concurrency).Validate(validatePositiveInt),
			huh.NewConfirm().
				Title("Loop through the request list").
				Description("When enabled, replays in a cycle until --total is reached").
				Value(&loop),
		),
		sharedOptsGroup(&shared),
	).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	args := []string{
		targetURL,
		"-file", file,
		"-concurrency", concurrency,
	}
	if total != "0" && total != "" {
		args = append(args, "-total", total)
	}
	if loop {
		args = append(args, "-loop")
	}
	args = append(args, shared.args()...)
	return dispatchInteractive(ctx, "replay", args, stdout, stderr)
}

func interactiveMethodSpray(ctx context.Context, stdout, stderr io.Writer) int {
	var (
		url         = ""
		methods     = "GET,POST,PUT,PATCH,DELETE,OPTIONS,HEAD"
		wordlist    = ""
		total       = "1000"
		concurrency = "20"
		timeoutSec  = "10"
	)
	shared := defaultSharedOpts()

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Base URL").
				Description("Scheme + host only; paths come from the wordlist").
				Placeholder("https://example.com").
				Value(&url).
				Validate(validateURL),
			huh.NewInput().
				Title("HTTP methods (comma-separated)").
				Description("Cross-product of methods × wordlist paths is sprayed").
				Value(&methods),
			huh.NewInput().
				Title("Wordlist path (empty = built-in)").
				Value(&wordlist),
			huh.NewInput().Title("Total requests").Value(&total).Validate(validatePositiveInt),
			huh.NewInput().Title("Concurrency (workers)").Value(&concurrency).Validate(validatePositiveInt),
			huh.NewInput().Title("Timeout (s)").Value(&timeoutSec).Validate(validatePositiveInt),
		),
		sharedOptsGroup(&shared),
	).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	args := []string{
		url,
		"-methods", methods,
		"-total", total,
		"-concurrency", concurrency,
		"-timeout", timeoutSec,
	}
	if wordlist != "" {
		args = append(args, "-wordlist", wordlist)
	}
	args = append(args, shared.args()...)
	return dispatchInteractive(ctx, "methodspray", args, stdout, stderr)
}

func dispatchInteractive(ctx context.Context, name string, args []string, stdout, stderr io.Writer) int {
	a, ok := attacks.Lookup(name)
	if !ok {
		fmt.Fprintf(stderr, "error: unknown attack %q\n", name)
		return 2
	}
	return runAttack(ctx, a, args, stdout, stderr)
}

func methodSelect(v *string) *huh.Select[string] {
	return huh.NewSelect[string]().Title("HTTP method").Options(httpMethodOptions()...).Value(v)
}

func httpMethodOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("GET", "GET"),
		huh.NewOption("POST", "POST"),
		huh.NewOption("PUT", "PUT"),
		huh.NewOption("PATCH", "PATCH"),
		huh.NewOption("DELETE", "DELETE"),
		huh.NewOption("HEAD", "HEAD"),
		huh.NewOption("OPTIONS", "OPTIONS"),
	}
}

func validatePositiveInt(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("must be an integer")
	}
	if n <= 0 {
		return fmt.Errorf("must be > 0")
	}
	return nil
}

func validateNonNegativeInt(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("must be an integer")
	}
	if n < 0 {
		return fmt.Errorf("must be >= 0")
	}
	return nil
}

func validatePositiveFloat(s string) error {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("must be a number")
	}
	if f <= 0 {
		return fmt.Errorf("must be > 0")
	}
	return nil
}
