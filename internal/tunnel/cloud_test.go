package tunnel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"local/tunnel-prototype/internal/ingress"
	"local/tunnel-prototype/internal/service"
)

func TestCloudIngressThroughBothGateways(t *testing.T) {
	bGateway, cfg, record, addr, count := setup(t, 0)
	password := "client-independent-password-0123456789"
	manager := service.New(service.Options{MaxConnections: 8, SessionTimeout: 3 * time.Second})
	a := httptest.NewServer(manager.Wrap((&ingress.Server{Passwords: []string{password}, Dial: func(ctx context.Context, target string) (net.Conn, error) {
		return Dial(ctx, "wss"+strings.TrimPrefix(bGateway.URL, "https")+"/tunnel", target, testToken, cfg, bGateway.Client())
	}}).Handler()))
	defer a.Close()
	aGateway, _ := gateway(t, a.URL, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "wss"+strings.TrimPrefix(aGateway.URL, "https")+"/proxy", &websocket.DialOptions{HTTPClient: aGateway.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	raw := websocket.NetConn(ctx, ws, websocket.MessageBinary)
	host, port, _ := net.SplitHostPort(addr)
	n, _ := strconv.Atoi(port)
	hash := sha256.Sum224([]byte(password))
	header := []byte(hex.EncodeToString(hash[:]) + "\r\n")
	header = append(header, 1, 1)
	header = append(header, net.ParseIP(host).To4()...)
	header = append(header, byte(n>>8), byte(n), '\r', '\n')
	// Fragment authentication across messages, then coalesce header tail + payload.
	for _, b := range header[:60] {
		if _, err := raw.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	payload := bytes.Repeat([]byte("cloud-chain-payload"), 60000)
	sent := make(chan error, 1)
	go func() {
		data := append(header[60:], payload...)
		for len(data) > 0 {
			n := len(data)
			if n > 16384 {
				n = 16384
			}
			if _, err := raw.Write(data[:n]); err != nil {
				sent <- err
				return
			}
			data = data[n:]
		}
		sent <- nil
	}()
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(raw, got); err != nil {
		t.Fatal(err)
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("cloud payload corrupted")
	}
	if count.Load() != 1 {
		t.Fatal("unexpected target connections")
	}
	if record.has([]byte(testToken)) || record.has([]byte("cloud-chain-payload")) {
		t.Fatal("Voyager gateway sees inner plaintext")
	}
	ws.CloseNow()
	deadline := time.Now().Add(time.Second)
	for manager.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if manager.Active() != 0 {
		t.Fatal("Gate retained disconnected client")
	}
}

func TestTokenRotation(t *testing.T) {
	st, ct := configs(t)
	addr, _ := echo(t, false)
	next := "rotated-token-0123456789abcdef01234567"
	b := httptest.NewTLSServer((&Server{TLSConfig: st, Tokens: []string{next, testToken}, Authorize: func(context.Context, string) (string, error) { return addr, nil }}).Handler())
	defer b.Close()
	for _, token := range []string{next, testToken} {
		c, err := dial(t, b, ct, addr, token)
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
	}
}
