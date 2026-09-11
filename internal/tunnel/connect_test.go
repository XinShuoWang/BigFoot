package tunnel

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"local/tunnel-prototype/internal/service"
)

// Catches loss of bytes buffered by HTTP parsing before the CONNECT hijack.
func TestHTTPConnectPreservesPipelinedBytes(t *testing.T) {
	g, cfg, _, addr, _ := setup(t, 0)
	a := httptest.NewServer(ConnectHandler(func(ctx context.Context, target string) (net.Conn, error) {
		return Dial(ctx, "wss"+strings.TrimPrefix(g.URL, "https")+"/tunnel", target, testToken, cfg, g.Client())
	}))
	defer a.Close()
	c, err := net.Dial("tcp", a.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\nearly-data", addr, addr)
	br := bufio.NewReader(c)
	res, err := http.ReadResponse(br, &http.Request{Method: "CONNECT"})
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	got := make([]byte, 10)
	if _, err = io.ReadFull(br, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "early-data" {
		t.Fatalf("lost bytes: %q", got)
	}
}

func TestBRejectsTLS12Peer(t *testing.T) {
	g, cfg, _, _, count := setup(t, 0)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "wss"+strings.TrimPrefix(g.URL, "https")+"/tunnel", &websocket.DialOptions{HTTPClient: g.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	raw := websocket.NetConn(ctx, ws, websocket.MessageBinary)
	defer raw.Close()
	cfg.MinVersion = tls.VersionTLS12
	cfg.MaxVersion = tls.VersionTLS12
	if err = tls.Client(raw, cfg).HandshakeContext(ctx); err == nil {
		t.Fatal("Voyager negotiated TLS 1.2")
	}
	if count.Load() != 0 {
		t.Fatal("contacted target")
	}
}

func TestClientCancellationClosesTarget(t *testing.T) {
	st, ct := configs(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		close(accepted)
		c.SetDeadline(time.Now().Add(3 * time.Second))
		var p [1]byte
		c.Read(p[:])
		close(closed)
	}()
	b := httptest.NewServer((&Server{TLSConfig: st, Token: testToken, Authorize: func(context.Context, string) (string, error) { return ln.Addr().String(), nil }}).Handler())
	defer b.Close()
	g, _ := gateway(t, b.URL, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, err := Dial(ctx, "wss"+strings.TrimPrefix(g.URL, "https")+"/tunnel", ln.Addr().String(), testToken, ct, g.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	<-accepted
	cancel()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("target was not closed after cancellation")
	}
}

func TestLocalActivityPreventsIdleExpiry(t *testing.T) {
	g, cfg, _, addr, _ := setup(t, 0)
	manager := service.New(service.Options{IdleTimeout: 80 * time.Millisecond, SessionTimeout: 2 * time.Second})
	a := httptest.NewServer(manager.Wrap(ConnectHandler(func(ctx context.Context, target string) (net.Conn, error) {
		return Dial(ctx, "wss"+strings.TrimPrefix(g.URL, "https")+"/tunnel", target, testToken, cfg, g.Client())
	})))
	defer a.Close()
	c, err := net.Dial("tcp", a.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", addr, addr)
	br := bufio.NewReader(c)
	res, err := http.ReadResponse(br, &http.Request{Method: "CONNECT"})
	if err != nil || res.StatusCode != 200 {
		t.Fatal("CONNECT failed")
	}
	for i := 0; i < 5; i++ {
		time.Sleep(30 * time.Millisecond)
		if _, err := c.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		if b, err := br.ReadByte(); err != nil || b != 'x' {
			t.Fatalf("active connection closed: %v", err)
		}
	}
}
