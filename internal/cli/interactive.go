package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/charmbracelet/huh"
)

func RunInteractive(ctx context.Context, stdout, stderr io.Writer) int {
	var attack string

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("ratelash — select attack").
				Options(
					huh.NewOption("flood       high-throughput request flood", "flood"),
					huh.NewOption("slowloris   hold connections open (slow header drip)", "slowloris"),
					huh.NewOption("spoof       X-Forwarded-For IP rotation with pacing", "spoof"),
					huh.NewOption("fuzz        path enumeration from wordlist", "fuzz"),
					huh.NewOption("headerbomb  oversized headers + growing body", "headerbomb"),
				).
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
	}
	return 0
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

	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Target URL").Placeholder("https://example.com").Value(&url).Validate(validateURL),
		huh.NewInput().Title("Total requests").Value(&total).Validate(validatePositiveInt),
		huh.NewInput().Title("Concurrency (workers)").Value(&concurrency).Validate(validatePositiveInt),
		methodSelect(&method),
		huh.NewInput().Title("Timeout (s)").Value(&timeoutSec).Validate(validatePositiveInt),
		huh.NewInput().Title("Request body (optional)").Value(&body),
	)).Run()

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
	return runFlood(ctx, args, stdout, stderr)
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

	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Target URL").Placeholder("https://example.com").Value(&url).Validate(validateURL),
		huh.NewInput().Title("Connections").Value(&connections).Validate(validatePositiveInt),
		huh.NewInput().Title("Header interval (s)").Value(&intervalSec).Validate(validatePositiveInt),
		huh.NewInput().Title("Hold duration (s)").Value(&holdSec).Validate(validatePositiveInt),
		huh.NewInput().Title("Dial timeout (s)").Value(&dialSec).Validate(validatePositiveInt),
		huh.NewConfirm().Title("Skip TLS verification").Value(&insecure),
	)).Run()

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
	return runSlowloris(ctx, args, stdout, stderr)
}

func interactiveSpoof(ctx context.Context, stdout, stderr io.Writer) int {
	var (
		url         = ""
		total       = "1000"
		concurrency = "20"
		method      = "GET"
		timeoutSec  = "10"
		ipPool      = ""
		pacing      = "none"
		rps         = "50"
		minDelayMs  = "0"
		maxDelayMs  = "50"
	)

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Target URL").Placeholder("https://example.com").Value(&url).Validate(validateURL),
			huh.NewInput().Title("Total requests").Value(&total).Validate(validatePositiveInt),
			huh.NewInput().Title("Concurrency (workers)").Value(&concurrency).Validate(validatePositiveInt),
			methodSelect(&method),
			huh.NewInput().Title("Timeout (s)").Value(&timeoutSec).Validate(validatePositiveInt),
		),
		huh.NewGroup(
			huh.NewInput().Title("IP pool (CIDR / file:path / comma list, optional)").Value(&ipPool),
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
	return runSpoof(ctx, args, stdout, stderr)
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

	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Base URL").Placeholder("https://example.com").Value(&url).Validate(validateURL),
		huh.NewInput().Title("Total requests").Value(&total).Validate(validatePositiveInt),
		huh.NewInput().Title("Concurrency (workers)").Value(&concurrency).Validate(validatePositiveInt),
		huh.NewInput().Title("Timeout (s)").Value(&timeoutSec).Validate(validatePositiveInt),
		huh.NewInput().Title("Wordlist path (empty = use built-in)").Value(&wordlist),
		huh.NewConfirm().Title("Cache bust (append _cb=<hex>)").Value(&cacheBust),
	)).Run()

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
	return runFuzz(ctx, args, stdout, stderr)
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
	return runHeaderbomb(ctx, args, stdout, stderr)
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
