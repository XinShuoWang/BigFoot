package tunnel

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const testToken = "only-inside-inner-tls-0123456789abcdef"

func configs(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"voyager.internal"}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kd, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kd}))
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}, &tls.Config{RootCAs: pool, ServerName: "voyager.internal", MinVersion: tls.VersionTLS13}
}

type recording struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (r *recording) add(p []byte) { r.mu.Lock(); defer r.mu.Unlock(); r.b.Write(p) }
func (r *recording) has(p []byte) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return bytes.Contains(r.b.Bytes(), p)
}

// Gateway terminates outer HTTPS and observes every decrypted WS payload.
func gateway(t *testing.T, backend string, limit time.Duration) (*httptest.Server, *recording) {
	t.Helper()
	rec := &recording{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		if limit > 0 {
			var stop context.CancelFunc
			ctx, stop = context.WithTimeout(ctx, limit)
			defer stop()
		}
		down, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer down.CloseNow()
		up, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(backend, "http")+r.URL.Path, nil)
		if err != nil {
			return
		}
		defer up.CloseNow()
		finished := make(chan struct{}, 2)
		pump := func(dst, src *websocket.Conn) {
			defer func() { finished <- struct{}{} }()
			for {
				typ, p, err := src.Read(ctx)
				if err != nil {
					return
				}
				rec.add(p)
				if err = dst.Write(ctx, typ, p); err != nil {
					return
				}
			}
		}
		down.SetReadLimit(1 << 20)
		up.SetReadLimit(1 << 20)
		go pump(up, down)
		go pump(down, up)
		<-finished
		cancel()
		down.CloseNow()
		up.CloseNow()
		<-finished
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func echo(t *testing.T, waitEOF bool) (string, *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	n := &atomic.Int32{}
	done := make(chan struct{})
	t.Cleanup(func() { ln.Close(); <-done })
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			n.Add(1)
			go func() {
				defer c.Close()
				c.SetDeadline(time.Now().Add(5 * time.Second))
				if waitEOF {
					p, _ := io.ReadAll(c)
					c.Write(append([]byte("reply:"), p...))
				} else {
					io.Copy(c, c)
				}
			}()
		}
	}()
	return ln.Addr().String(), n
}

func setup(t *testing.T, limit time.Duration) (*httptest.Server, *tls.Config, *recording, string, *atomic.Int32) {
	t.Helper()
	serverTLS, clientTLS := configs(t)
	addr, count := echo(t, false)
	b := &Server{TLSConfig: serverTLS, Token: testToken, SessionTimeout: 3 * time.Second, Authorize: func(ctx context.Context, target string) (string, error) {
		if target != addr {
			return "", fmt.Errorf("blocked")
		}
		return target, nil
	}}
	backend := httptest.NewServer(b.Handler())
	t.Cleanup(backend.Close)
	gw, rec := gateway(t, backend.URL, limit)
	return gw, clientTLS, rec, addr, count
}

func dial(t *testing.T, g *httptest.Server, cfg *tls.Config, addr, token string) (net.Conn, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	t.Cleanup(cancel)
	return Dial(ctx, "wss"+strings.TrimPrefix(g.URL, "https")+"/tunnel", addr, token, cfg, g.Client())
}

// Catches missing encryption, dropped or reordered multi-frame data.
func TestRoundTripGatewayCannotReadPayload(t *testing.T) {
	g, cfg, rec, addr, _ := setup(t, 0)
	c, err := dial(t, g, cfg, addr, testToken)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p := bytes.Repeat([]byte("private-application-payload-"), 40000)
	done := make(chan error, 1)
	go func() { _, err := c.Write(p); done <- err }()
	got := make([]byte, len(p))
	if _, err = io.ReadFull(c, got); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, p) {
		t.Fatal("payload corrupted")
	}
	if rec.has([]byte(testToken)) || rec.has([]byte("private-application-payload-")) {
		t.Fatal("gateway saw plaintext")
	}
}

// Removing auth would open the target and allow the dial.
func TestWrongTokenDoesNotReachTarget(t *testing.T) {
	g, cfg, _, addr, count := setup(t, 0)
	c, err := dial(t, g, cfg, addr, "wrong-password-01234567890123456789")
	if c != nil {
		c.Close()
	}
	if err == nil {
		t.Fatal("accepted wrong token")
	}
	if count.Load() != 0 {
		t.Fatal("target contacted before auth")
	}
}

func TestRejectWrongCAAndHostname(t *testing.T) {
	for _, kind := range []string{"CA", "hostname"} {
		t.Run(kind, func(t *testing.T) {
			g, cfg, _, addr, count := setup(t, 0)
			if kind == "CA" {
				cfg.RootCAs = x509.NewCertPool()
			} else {
				cfg.ServerName = "impostor.internal"
			}
			c, err := dial(t, g, cfg, addr, testToken)
			if c != nil {
				c.Close()
			}
			if err == nil {
				t.Fatal("accepted untrusted peer")
			}
			if count.Load() != 0 {
				t.Fatal("target contacted")
			}
		})
	}
}

func TestRejectUnencryptedOuterTransport(t *testing.T) {
	_, cfg := configs(t)
	c, err := Dial(context.Background(), "ws://127.0.0.1:1/tunnel", "example.org:443", testToken, cfg, nil)
	if c != nil {
		c.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "wss") {
		t.Fatalf("want explicit wss requirement, got %v", err)
	}
}

func TestPublicTargetPolicy(t *testing.T) {
	for _, target := range []string{"127.0.0.1:443", "[::1]:80", "169.254.169.254:80", "10.0.0.1:80", "localhost:80", "example.org:0"} {
		if _, err := PublicTarget(context.Background(), target); err == nil {
			t.Errorf("allowed %s", target)
		}
	}
}

func TestTargetDeniedAfterAuthentication(t *testing.T) {
	g, cfg, _, _, count := setup(t, 0)
	c, err := dial(t, g, cfg, "127.0.0.1:9", testToken)
	if c != nil {
		c.Close()
	}
	if err == nil {
		t.Fatal("accepted forbidden target")
	}
	if count.Load() != 0 {
		t.Fatal("contacted target")
	}
}

func TestWebsiteFallback(t *testing.T) {
	cfg, _ := configs(t)
	b := httptest.NewServer((&Server{TLSConfig: cfg, Token: testToken}).Handler())
	defer b.Close()
	for _, path := range []string{"/", "/tunnel", "/unknown"} {
		res, err := http.Get(b.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || !bytes.Contains(body, []byte("<!doctype html>")) {
			t.Fatal("ordinary request did not get website")
		}
	}
}

func TestHalfClosePreservesResponse(t *testing.T) {
	st, ct := configs(t)
	addr, _ := echo(t, true)
	b := httptest.NewServer((&Server{TLSConfig: st, Token: testToken, Authorize: func(context.Context, string) (string, error) { return addr, nil }}).Handler())
	defer b.Close()
	g, _ := gateway(t, b.URL, 0)
	c, err := dial(t, g, ct, addr, testToken)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Write([]byte("request-body"))
	if err = c.(interface{ CloseWrite() error }).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(c)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "reply:request-body" {
		t.Fatalf("got %q", got)
	}
}

func TestGatewayTimeoutAndFreshConnection(t *testing.T) {
	g, cfg, _, addr, _ := setup(t, 250*time.Millisecond)
	c, err := dial(t, g, cfg, addr, testToken)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	start := time.Now()
	buf := make([]byte, 1)
	if _, err = c.Read(buf); err == nil {
		t.Fatal("expired connection stayed open")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("timeout cleanup too slow")
	}
	next, err := dial(t, g, cfg, addr, testToken)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	next.Write([]byte("x"))
	if _, err = io.ReadFull(next, buf); err != nil || buf[0] != 'x' {
		t.Fatalf("fresh connection failed: %v", err)
	}
}

func TestTLS12Rejected(t *testing.T) {
	g, cfg, _, addr, count := setup(t, 0)
	cfg.MinVersion = tls.VersionTLS12
	cfg.MaxVersion = tls.VersionTLS12
	c, err := dial(t, g, cfg, addr, testToken)
	if c != nil {
		c.Close()
	}
	if err == nil {
		t.Fatal("TLS 1.2 accepted")
	}
	if count.Load() != 0 {
		t.Fatal("target contacted")
	}
}
