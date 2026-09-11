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
)

func TestRejectSpecialPurposeTargets(t *testing.T) {
	for _, target := range []string{"0.0.0.1:443", "[100::1]:443", "[2001:2::1]:443", "192.88.99.2:443", "[64:ff9b:1::1]:443", "[3fff::1]:443", "[5f00::1]:443"} {
		got, err := PublicTarget(context.Background(), target)
		if err == nil {
			t.Errorf("non-public target accepted: %s -> %s", target, got)
		}
	}
}

func TestAllowOrdinaryPublicTargets(t *testing.T) {
	for _, target := range []string{"8.8.8.8:443", "[2606:4700:4700::1111]:443", "[2001:4860:4860::8888]:443"} {
		got, err := PublicTarget(context.Background(), target)
		if err != nil || got != target {
			t.Errorf("public target %s: got %q, error %v", target, got, err)
		}
	}
}

func TestRejectInsecureDefaultOuterClient(t *testing.T) {
	st, ct := configs(t)
	addr, _ := echo(t, false)
	b := httptest.NewTLSServer((&Server{TLSConfig: st, Token: testToken, Authorize: func(context.Context, string) (string, error) { return addr, nil }}).Handler())
	defer b.Close()
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	defer tr.CloseIdleConnections()
	// These subtests modify HTTP globals and must remain sequential.
	for _, kind := range []string{"explicit", "default-client", "default-transport", "both-defaults"} {
		t.Run(kind, func(t *testing.T) {
			originalClient, originalTransport := http.DefaultClient, http.DefaultTransport
			defer func() { http.DefaultClient, http.DefaultTransport = originalClient, originalTransport }()
			var client *http.Client
			switch kind {
			case "explicit":
				client = &http.Client{Transport: tr}
			case "default-client":
				http.DefaultClient = &http.Client{Transport: tr}
			case "default-transport":
				client = &http.Client{}
				http.DefaultTransport = tr
			case "both-defaults":
				http.DefaultClient = &http.Client{}
				http.DefaultTransport = tr
			}
			c, err := Dial(context.Background(), "wss"+strings.TrimPrefix(b.URL, "https")+"/tunnel", addr, testToken, ct, client)
			if c != nil {
				c.Close()
			}
			if err == nil {
				t.Fatal("outer untrusted certificate accepted")
			}
		})
	}
}

func TestSessionTimeoutAfterFINReleasesA(t *testing.T) {
	st, ct := configs(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	received := make(chan error, 1)
	go func() {
		p, e := ln.Accept()
		if e != nil {
			received <- e
			return
		}
		defer p.Close()
		p.SetDeadline(time.Now().Add(3 * time.Second))
		if e = p.(*net.TCPConn).CloseWrite(); e != nil {
			received <- e
			return
		}
		var payload [9]byte
		_, e = io.ReadFull(p, payload[:])
		if e == nil && string(payload[:]) != "after-fin" {
			e = fmt.Errorf("unexpected payload: %q", payload)
		}
		received <- e
		io.Copy(io.Discard, p)
	}()
	b := httptest.NewTLSServer((&Server{TLSConfig: st, Token: testToken, SessionTimeout: 500 * time.Millisecond, Authorize: func(context.Context, string) (string, error) { return ln.Addr().String(), nil }}).Handler())
	defer b.Close()
	done := make(chan struct{})
	h := ConnectHandler(func(ctx context.Context, target string) (net.Conn, error) {
		return Dial(ctx, "wss"+strings.TrimPrefix(b.URL, "https")+"/tunnel", target, testToken, ct, b.Client())
	})
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { defer close(done); h.ServeHTTP(w, r) }))
	defer a.Close()
	c, err := net.Dial("tcp", a.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err = fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", ln.Addr(), ln.Addr()); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	res, e := http.ReadResponse(br, &http.Request{Method: "CONNECT"})
	if e != nil || res.StatusCode != 200 {
		t.Fatalf("CONNECT: %v", e)
	}
	if _, e = br.ReadByte(); e != io.EOF {
		t.Fatalf("want FIN EOF: %v", e)
	}
	// Receiving FIN must not prematurely close the reverse direction.
	if _, err = c.Write([]byte("after-fin")); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-received:
		if err != nil {
			t.Fatalf("reverse traffic after FIN: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reverse traffic after FIN stalled")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Gate handler retained after Voyager FIN then session timeout")
	}
}
