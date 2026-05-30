package attacks_test

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/conantorreswf/limithit/internal/attacks"
	_ "github.com/conantorreswf/limithit/internal/attacks/all"
	"github.com/conantorreswf/limithit/internal/client"
)

type serverFactory func(h http.Handler) *httptest.Server

func plainServer(h http.Handler) *httptest.Server {
	return httptest.NewServer(h)
}

func tlsH2Server(h http.Handler) *httptest.Server {
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	return srv
}

type integrationConfig struct {
	extraArgs []string
	mkServer  serverFactory
}

// attackCases maps attack name → per-attack test overrides.
// Attacks not listed use plain httptest with no extra flags.
var attackCases = map[string]integrationConfig{
	"gzipbomb":   {extraArgs: []string{"--i-understand", "--expanded-mb", "1"}},
	"h2flood":    {extraArgs: []string{"--insecure", "--connections", "1", "--streams", "3"}, mkServer: tlsH2Server},
	"headerbomb": {extraArgs: []string{"--header-count", "3", "--header-size", "32", "--body-start", "64", "--body-max", "256"}},
	"slowloris":  {extraArgs: []string{"--connections", "2", "--hold", "1", "--header-interval", "2"}},
	"spoof":      {extraArgs: []string{"--ip-pool", "10.0.0.1,10.0.0.2"}},
	"wsflood":    {extraArgs: []string{"--connections", "2", "--hold", "1"}},
}

func TestAttackIntegration(t *testing.T) {
	all := attacks.All()
	if len(all) == 0 {
		t.Fatal("no attacks registered; check imports")
	}

	// Single handler used by all test servers; supports plain HTTP and WS upgrades.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") == "websocket" {
			wsUpgrade(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	})

	for _, a := range all {
		a := a
		t.Run(a.Name(), func(t *testing.T) {
			t.Parallel()

			cfg := attackCases[a.Name()]
			mk := cfg.mkServer
			if mk == nil {
				mk = plainServer
			}
			srv := mk(handler)
			defer srv.Close()

			extraArgs := cfg.extraArgs
			if a.Name() == "replay" {
				f, err := os.CreateTemp(t.TempDir(), "replay-*.txt")
				if err != nil {
					t.Fatalf("create temp file: %v", err)
				}
				_, _ = fmt.Fprintf(f, "GET %s\n", srv.URL)
				f.Close()
				extraArgs = append(extraArgs, "--file", f.Name(), "--loop")
			}

			attack, ok := attacks.Lookup(a.Name())
			if !ok {
				t.Fatalf("Lookup(%q) failed", a.Name())
			}

			fs := flag.NewFlagSet(a.Name(), flag.ContinueOnError)
			attack.Flags(fs)
			if err := fs.Parse(extraArgs); err != nil {
				t.Fatalf("flag parse: %v", err)
			}
			if err := attack.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}

			hc := srv.Client()
			if hc == nil {
				hc = client.New(client.Config{Timeout: 5 * time.Second}, 4)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			rep, err := attack.Run(ctx, attacks.Base{
				URL:    srv.URL,
				Client: hc,
				Common: attacks.CommonOpts{
					Total:       5,
					Concurrency: 2,
					Timeout:     5 * time.Second,
				},
			})
			if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
			if rep == nil {
				t.Fatal("Run returned nil report")
			}
			if rep.String() == "" {
				t.Fatal("Report.String() is empty")
			}
		})
	}
}

// wsUpgrade responds with WebSocket 101 and holds until the client closes or
// the read deadline fires, to allow wsflood connections to register as established.
func wsUpgrade(w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()

	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + wsAccept(r.Header.Get("Sec-WebSocket-Key")) + "\r\n\r\n"
	_, _ = bufrw.WriteString(resp)
	_ = bufrw.Flush()

	// Block until client closes or read deadline fires.
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, _ = conn.Read(make([]byte, 1))
}

func wsAccept(key string) string {
	h := sha1.New()
	_, _ = h.Write([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
