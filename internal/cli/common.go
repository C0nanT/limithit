package cli

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type HeaderFlag struct {
	Headers http.Header
}

func (h *HeaderFlag) String() string {
	if h.Headers == nil {
		return ""
	}
	parts := make([]string, 0, len(h.Headers))
	for k, vs := range h.Headers {
		for _, v := range vs {
			parts = append(parts, fmt.Sprintf("%s: %s", k, v))
		}
	}
	return strings.Join(parts, ", ")
}

func (h *HeaderFlag) Set(s string) error {
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return errors.New(`header must be "Key: Value"`)
	}
	key := strings.TrimSpace(s[:idx])
	val := strings.TrimSpace(s[idx+1:])
	if key == "" {
		return errors.New("header key is empty")
	}
	if h.Headers == nil {
		h.Headers = make(http.Header)
	}
	h.Headers.Add(key, val)
	return nil
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

// firstPositional extracts the URL passed as the first non-flag argument,
// supporting both "limithit flood https://x" and "limithit flood --url https://x".
func firstPositional(fs *flag.FlagSet) string {
	if fs.NArg() > 0 {
		return fs.Arg(0)
	}
	return ""
}

// extractURLArg scans args for the first http(s) URL token and returns it
// along with the remaining args (suitable for fs.Parse). This lets users put
// the URL anywhere: "limithit flood URL --flag" or "limithit flood --flag URL".
func extractURLArg(args []string) (string, []string) {
	for i, a := range args {
		if strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") {
			rest := make([]string, 0, len(args)-1)
			if i > 0 && args[i-1] == "--url" {
				// Strip --url flag name too so flag.Parse doesn't consume the next token as its value.
				rest = append(rest, args[:i-1]...)
			} else {
				rest = append(rest, args[:i]...)
			}
			rest = append(rest, args[i+1:]...)
			return a, rest
		}
	}
	return "", args
}
