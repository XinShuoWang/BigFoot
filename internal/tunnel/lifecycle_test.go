package tunnel

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRejectOuterRedirectDowngrade(t *testing.T) {
	st, ct := configs(t)
	addr, count := echo(t, false)
	b := httptest.NewServer((&Server{TLSConfig: st, Token: testToken, Authorize: func(context.Context, string) (string, error) { return addr, nil }}).Handler())
	defer b.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, b.URL+"/tunnel", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	c, err := Dial(context.Background(), "wss"+strings.TrimPrefix(redirect.URL, "https")+"/tunnel", addr, testToken, ct, redirect.Client())
	if c != nil {
		c.Close()
	}
	if err == nil {
		t.Fatal("accepted plaintext redirected WS")
	}
	if count.Load() != 0 {
		t.Fatal("target reached after insecure redirect")
	}
}

func TestDisconnectAfterFINClosesVoyagerHandler(t *testing.T) {
	st, ct := configs(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	peerCh := make(chan net.Conn, 1)
	go func() {
		p, err := ln.Accept()
		if err == nil {
			peerCh <- p
		}
	}()
	done := make(chan struct{})
	handler := (&Server{TLSConfig: st, Token: testToken, SessionTimeout: 3 * time.Second, Authorize: func(context.Context, string) (string, error) { return ln.Addr().String(), nil }}).Handler()
	b := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { defer close(done); handler.ServeHTTP(w, r) }))
	defer b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	c, err := Dial(ctx, "wss"+strings.TrimPrefix(b.URL, "https")+"/tunnel", ln.Addr().String(), testToken, ct, b.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	peer := <-peerCh
	defer peer.Close()
	peer.SetDeadline(time.Now().Add(time.Second))
	if err = c.(interface{ CloseWrite() error }).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if _, err = io.ReadAll(peer); err != nil {
		t.Fatal(err)
	}
	c.Close()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Voyager handler retained after FIN and disconnect")
	}
}
