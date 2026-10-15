package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"local/tunnel-prototype/internal/identity"
	"local/tunnel-prototype/internal/routing"
	"local/tunnel-prototype/internal/tunnel"
)

func testIdentity(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test Voyager"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: raw})
}

// Detects actual routing errors, credential mixing, early dialing before auth,
// and silent fallback: each real Voyager returns its own marker over inner TLS.
func TestRoutedGateReachesIndependentVoyagers(t *testing.T) {
	env := map[string]string{"BIGFOOT_CLIENT_PASSWORD": "client-password-01234567890123456789"}
	route := routing.Config{Default: "hk", Nodes: map[string]routing.Node{}, Rules: []routing.RuleSet{
		{Source: "jp.txt", Voyager: "jp", Lines: []string{"DOMAIN-SUFFIX,example.jp"}},
		{Source: "sg.txt", Voyager: "sg", Lines: []string{"DOMAIN-SUFFIX,example.sg"}},
	}}
	counts := map[string]*atomic.Int32{}
	outerCA := []byte{}
	for _, name := range []string{"hk", "jp", "sg"} {
		name := name
		ca, key := testIdentity(t)
		issuer, err := identity.New(ca, key, "voyager.internal")
		if err != nil {
			t.Fatal(err)
		}
		token := "token-for-" + name + "-012345678901234567890123456789"
		tokenEnv := "BIGFOOT_NODE_" + strings.ToUpper(name) + "_TOKEN"
		caEnv := "BIGFOOT_NODE_" + strings.ToUpper(name) + "_CA_PEM"
		env[tokenEnv] = token
		env[caEnv] = string(ca)
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				go func() {
					defer conn.Close()
					conn.SetDeadline(time.Now().Add(3 * time.Second))
					conn.Write([]byte(name))
					io.Copy(io.Discard, conn)
				}()
			}
		}()
		count := &atomic.Int32{}
		counts[name] = count
		backend := httptest.NewTLSServer((&tunnel.Server{TLSConfig: issuer.TLSConfig(), Token: token, Authorize: func(context.Context, string) (string, error) { count.Add(1); return ln.Addr().String(), nil }}).Handler())
		t.Cleanup(backend.Close)
		outerCA = append(outerCA, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: backend.Certificate().Raw})...)
		route.Nodes[name] = routing.Node{URL: "wss" + strings.TrimPrefix(backend.URL, "https") + "/tunnel", TokenEnv: tokenEnv, CAEnv: caEnv}
	}
	env["BIGFOOT_OUTER_CA_PEM"] = string(outerCA)
	file := filepath.Join(t.TempDir(), "routing.json")
	save := func() {
		raw, _ := json.Marshal(route)
		if err := os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	save()
	handler := func() *httptest.Server {
		c, err := parseConfig([]string{"-mode", "gate", "-behind-tls-proxy", "-routing-config", file}, func(k string) string { return env[k] })
		if err != nil {
			t.Fatal(err)
		}
		h, err := makeHandler(c)
		if err != nil {
			t.Fatal(err)
		}
		gate := httptest.NewTLSServer(h)
		t.Cleanup(gate.Close)
		return gate
	}
	request := func(gate *httptest.Server, host, password string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		ws, _, err := websocket.Dial(ctx, "wss"+strings.TrimPrefix(gate.URL, "https")+"/proxy", &websocket.DialOptions{HTTPClient: gate.Client()})
		if err != nil {
			return "", err
		}
		defer ws.CloseNow()
		conn := websocket.NetConn(ctx, ws, websocket.MessageBinary)
		hash := sha256.Sum224([]byte(password))
		header := []byte(hex.EncodeToString(hash[:]) + "\r\n")
		header = append(header, 1, 3, byte(len(host)))
		header = append(header, []byte(host)...)
		header = append(header, 1, 187, '\r', '\n')
		if _, err = conn.Write(header); err != nil {
			return "", err
		}
		marker := make([]byte, 2)
		_, err = io.ReadFull(conn, marker)
		return string(marker), err
	}
	gate := handler()
	for _, tt := range []struct{ host, want string }{{"a.example.jp", "jp"}, {"example.sg", "sg"}, {"other.example", "hk"}} {
		got, err := request(gate, tt.host, env["BIGFOOT_CLIENT_PASSWORD"])
		if err != nil || got != tt.want {
			t.Fatalf("%s got %s (%v)", tt.host, got, err)
		}
	}
	total := func() int32 { return counts["hk"].Load() + counts["jp"].Load() + counts["sg"].Load() }
	before := total()
	if _, err := request(gate, "example.jp", "wrong-password-01234567890123456789"); err == nil {
		t.Fatal("wrong password accepted")
	}
	if total() != before {
		t.Fatal("dialed before authentication")
	}
	// A Tokyo token cannot authenticate to Hong Kong, and must not fall back.
	originalToken := env["BIGFOOT_NODE_JP_TOKEN"]
	env["BIGFOOT_NODE_JP_TOKEN"] = env["BIGFOOT_NODE_HK_TOKEN"]
	badGate := handler()
	before = total()
	if _, err := request(badGate, "example.jp", env["BIGFOOT_CLIENT_PASSWORD"]); err == nil {
		t.Fatal("mixed token accepted")
	}
	if total() != before {
		t.Fatal("failed node fell back or reached target")
	}
	// Trusting the other node must fail even though both certify voyager.internal.
	env["BIGFOOT_NODE_JP_TOKEN"] = originalToken
	strangerCA, _ := testIdentity(t)
	env["BIGFOOT_NODE_JP_CA_PEM"] = string(strangerCA)
	badGate = handler()
	if _, err := request(badGate, "example.jp", env["BIGFOOT_CLIENT_PASSWORD"]); err == nil {
		t.Fatal("wrong node identity accepted")
	}
	if total() != before {
		t.Fatal("untrusted node reached a target")
	}
	// Changing only the default changes the path for unmatched domains.
	route.Default = "sg"
	save()
	gate = handler()
	got, err := request(gate, "unmatched.example", env["BIGFOOT_CLIENT_PASSWORD"])
	if err != nil || got != "sg" {
		t.Fatal(fmt.Sprintf("default not applied: %s %v", got, err))
	}
	env["BIGFOOT_NODE_JP_CA_PEM"] = env["BIGFOOT_NODE_HK_CA_PEM"]
	c, err := parseConfig([]string{"-mode", "gate", "-behind-tls-proxy", "-routing-config", file}, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = makeHandler(c); err == nil {
		t.Fatal("shared Voyager identity accepted")
	}

}
