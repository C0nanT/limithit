package safety

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
)

// amplificationAttacks is the set of attack names that require --i-understand.
var amplificationAttacks = map[string]bool{
	"gzipbomb": true,
}

// Opts carries the safety-related flags parsed by the CLI.
type Opts struct {
	IUnderstand  bool     // --i-understand
	AllowTargets []string // --allow-target values; "*" means any
	MaxRPS       float64  // --max-rps (0 = uncapped)
}

// Confirm runs pre-flight safety checks. It returns a hard error for
// affirmation failures and writes warnings for non-fatal concerns to warn.
func Confirm(attackName, target string, opts Opts, warn io.Writer) error {
	// Amplification attacks require explicit affirmation.
	if amplificationAttacks[attackName] && !opts.IUnderstand {
		return fmt.Errorf(
			"attack %q is a destructive amplification attack — it may crash or OOM the target\n"+
				"  add --i-understand to confirm you have explicit authorisation to run this test",
			attackName,
		)
	}

	// Warn when the target is not loopback, unless --allow-target covers it.
	if !isLoopback(target) && !isAllowed(target, opts.AllowTargets) {
		host := hostOf(target)
		fmt.Fprintf(warn,
			"WARNING: target %q is not localhost — ensure you have written authorisation\n"+
				"  use --allow-target %q to suppress this warning\n",
			host, host,
		)
	}

	return nil
}

func isLoopback(rawURL string) bool {
	host := hostOf(rawURL)
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if !net.ParseIP(a).IsLoopback() {
			return false
		}
	}
	return len(addrs) > 0
}

func isAllowed(rawURL string, allowed []string) bool {
	for _, a := range allowed {
		if a == "*" {
			return true
		}
		if strings.EqualFold(hostOf(rawURL), a) {
			return true
		}
	}
	return false
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	h := u.Hostname()
	if h == "" {
		return rawURL
	}
	return h
}
