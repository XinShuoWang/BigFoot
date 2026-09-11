package ingress

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const password = "client-secret-0123456789abcdef01234567"

func header(secret string, address []byte) []byte {
	h := sha256.Sum224([]byte(secret))
	p := []byte(hex.EncodeToString(h[:]) + "\r\n")
	return append(append(p, address...), '\r', '\n')
}

func TestTrojanRequest(t *testing.T) {
	for _, tc := range []struct {
		name    string
		address []byte
		want    string
	}{
		{"ipv4", []byte{1, 1, 8, 8, 8, 8, 1, 187}, "8.8.8.8:443"},
		{"domain", append([]byte{1, 3, 11}, append([]byte("example.com"), 0, 80)...), "example.com:80"},
		{"ipv6", []byte{1, 4, 0x26, 6, 0x47, 0, 0x47, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x11, 0x11, 1, 187}, "[2606:4700:4700::1111]:443"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := bytes.NewReader(append(header(password, tc.address), []byte("first-payload")...))
			got, err := readRequest(r, []string{password})
			if err != nil || got != tc.want {
				t.Fatalf("%q %v", got, err)
			}
			rest, _ := io.ReadAll(r)
			if string(rest) != "first-payload" {
				t.Fatal("consumed payload")
			}
		})
	}
}

func TestTrojanRejectsInvalidRequests(t *testing.T) {
	good := header(password, []byte{1, 1, 8, 8, 8, 8, 1, 187})
	for name, p := range map[string][]byte{
		"password":   header("incorrect", []byte{1, 1, 8, 8, 8, 8, 1, 187}),
		"udp":        header(password, []byte{3, 1, 8, 8, 8, 8, 1, 187}),
		"zero-port":  header(password, []byte{1, 1, 8, 8, 8, 8, 0, 0}),
		"empty-host": header(password, []byte{1, 3, 0, 1, 187}),
		"bad-atyp":   header(password, []byte{1, 8, 0, 1, 187}),
		"truncated":  good[:len(good)-1],
		"delimiter":  append(append([]byte{}, good[:56]...), []byte("xx")...),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readRequest(bytes.NewReader(p), []string{password}); err == nil {
				t.Fatal("accepted invalid request")
			}
		})
	}
}

func TestAuthenticationBeforeDialAndHandshakeTimeout(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewTLSServer((&Server{Passwords: []string{password}, HandshakeTimeout: 50 * time.Millisecond, Dial: func(context.Context, string) (net.Conn, error) { calls.Add(1); return nil, io.EOF }}).Handler())
	defer s.Close()
	for _, p := range [][]byte{header("wrong", []byte{1, 1, 8, 8, 8, 8, 0, 80}), []byte("incomplete")} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		ws, _, err := websocket.Dial(ctx, "wss"+strings.TrimPrefix(s.URL, "https")+"/proxy", &websocket.DialOptions{HTTPClient: s.Client()})
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		ws.Write(ctx, websocket.MessageBinary, p)
		if _, _, err = ws.Read(ctx); err == nil {
			t.Error("invalid client stayed connected")
		}
		ws.CloseNow()
		cancel()
	}
	if calls.Load() != 0 {
		t.Fatal("dial before authentication")
	}
}

func FuzzTrojanRequest(f *testing.F) {
	f.Add(header(password, []byte{1, 1, 8, 8, 8, 8, 1, 187}))
	f.Add([]byte("invalid"))
	f.Fuzz(func(t *testing.T, p []byte) { _, _ = readRequest(bytes.NewReader(p), []string{password}) })
}
