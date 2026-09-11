// Package tunnel carries authenticated TCP streams over WSS and inner TLS.
package tunnel

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"local/tunnel-prototype/internal/service"
)

const handshakeLimit = 10 * time.Second

type Server struct {
	TLSConfig      *tls.Config
	Token          string
	Tokens         []string // Rotation keys; Token remains supported for embedded callers.
	SessionTimeout time.Duration
	// Authorize resolves and validates a target, returning the exact numeric IP:port
	// to dial. Nil uses PublicTarget. Do not resolve again after authorization.
	Authorize func(context.Context, string) (string, error)
}

type request struct {
	Version int    `json:"version"`
	Token   string `json:"token"`
	Target  string `json:"target"`
}
type response struct {
	OK bool `json:"ok"`
}

func writeJSON(w io.Writer, v any) error {
	p, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(p) > 8192 {
		return errors.New("request too large")
	}
	h := make([]byte, 4)
	binary.BigEndian.PutUint32(h, uint32(len(p)))
	if err = writeAll(w, h); err != nil {
		return err
	}
	return writeAll(w, p)
}
func readJSON(r io.Reader, v any) error {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(h[:])
	if n == 0 || n > 8192 {
		return errors.New("invalid request size")
	}
	p := make([]byte, int(n))
	if _, err := io.ReadFull(r, p); err != nil {
		return err
	}
	return json.Unmarshal(p, v)
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tunnel" || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, "<!doctype html><html lang=\"en\"><title>Welcome</title><body><h1>Welcome</h1><p>This website is available.</p></body></html>")
			return
		}
		tokens := append([]string(nil), s.Tokens...)
		if s.Token != "" {
			tokens = append(tokens, s.Token)
		}
		if s.TLSConfig == nil || len(tokens) == 0 {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		lifetime := s.SessionTimeout
		if lifetime <= 0 {
			lifetime = 15 * time.Minute
		}
		ctx, cancel := context.WithTimeout(r.Context(), lifetime)
		defer cancel()
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
		if err != nil {
			return
		}
		defer ws.CloseNow()
		raw := websocket.NetConn(ctx, ws, websocket.MessageBinary)
		ws.SetReadLimit(1 << 20)
		defer raw.Close()
		cfg := s.TLSConfig.Clone()
		cfg.MinVersion = tls.VersionTLS13
		inner := tls.Server(raw, cfg)
		inner.SetDeadline(time.Now().Add(handshakeLimit))
		if err = inner.HandshakeContext(ctx); err != nil {
			return
		}
		var req request
		if err = readJSON(inner, &req); err != nil {
			return
		}
		got := sha256.Sum256([]byte(req.Token))
		valid := 0
		for _, token := range tokens {
			want := sha256.Sum256([]byte(token))
			if len(token) >= 32 {
				valid |= subtle.ConstantTimeCompare(got[:], want[:])
			}
		}
		if req.Version != 1 || valid != 1 {
			writeJSON(inner, response{})
			return
		}
		authorize := s.Authorize
		if authorize == nil {
			authorize = PublicTarget
		}
		dialCtx, stop := context.WithTimeout(ctx, handshakeLimit)
		defer stop()
		target, err := authorize(dialCtx, req.Target)
		if err != nil {
			writeJSON(inner, response{})
			return
		}
		peer, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", target)
		if err != nil {
			writeJSON(inner, response{})
			return
		}
		defer peer.Close()
		if err = writeJSON(inner, response{OK: true}); err != nil {
			return
		}
		inner.SetDeadline(time.Time{})
		stream := &Conn{Conn: inner, closeFn: func() error { peer.Close(); ws.CloseNow(); return raw.Close() }}
		defer stream.Close()
		stopClose := context.AfterFunc(ctx, func() { stream.Close(); peer.Close() })
		defer stopClose()
		relay(service.Track(ctx, stream), peer)
	})
}

// Dial authenticates the outer WSS endpoint AND the independent inner TLS peer.
// ctx bounds the complete returned connection lifetime, not just the handshake.
func Dial(ctx context.Context, endpoint, target, token string, cfg *tls.Config, client *http.Client) (net.Conn, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "wss" || u.Host == "" || u.User != nil {
		return nil, errors.New("a wss URL without userinfo is required")
	}
	if cfg == nil || cfg.InsecureSkipVerify || cfg.ServerName == "" {
		return nil, errors.New("verified inner TLS with a server name is required")
	}
	if len(token) < 32 {
		return nil, errors.New("token must contain at least 32 bytes")
	}
	// Pin the configured endpoint. In particular, never follow HTTPS -> HTTP
	// redirects inside websocket.Dial. Do not mutate the caller's HTTP client.
	baseClient := http.DefaultClient
	if client != nil {
		baseClient = client
	}
	strictClient := *baseClient
	transport := strictClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if tr, ok := transport.(*http.Transport); ok && tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
		return nil, errors.New("outer TLS verification must not be disabled")
	}
	strictClient.Transport = transport
	strictClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("WebSocket endpoint redirects are not allowed")
	}
	dialCtx, cancel := context.WithCancel(ctx)
	timer := time.AfterFunc(handshakeLimit, cancel)
	ws, resp, err := websocket.Dial(dialCtx, endpoint, &websocket.DialOptions{HTTPClient: &strictClient, CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		timer.Stop()
		cancel()
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return nil, fmt.Errorf("outer connection: %w", err)
	}
	raw := websocket.NetConn(dialCtx, ws, websocket.MessageBinary)
	ws.SetReadLimit(1 << 20)
	// NetConn cancellation only interrupts active reads/writes. An idle caller
	// may have neither, so close the physical connection on context cancellation.
	stopClose := context.AfterFunc(dialCtx, func() { ws.CloseNow() })
	closeFn := func() error { stopClose(); cancel(); ws.CloseNow(); return raw.Close() }
	fail := func(err error) (net.Conn, error) { timer.Stop(); closeFn(); return nil, err }
	innerCfg := cfg.Clone()
	innerCfg.MinVersion = tls.VersionTLS13
	inner := tls.Client(raw, innerCfg)
	if err = inner.HandshakeContext(dialCtx); err != nil {
		return fail(fmt.Errorf("inner TLS: %w", err))
	}
	if err = writeJSON(inner, request{Version: 1, Token: token, Target: target}); err != nil {
		return fail(err)
	}
	var result response
	if err = readJSON(inner, &result); err != nil {
		return fail(err)
	}
	if !result.OK {
		return fail(errors.New("authentication or target rejected"))
	}
	if !timer.Stop() {
		return fail(context.DeadlineExceeded)
	}
	return &Conn{Conn: inner, closeFn: closeFn}, nil
}

// PublicTarget pins one DNS result, rejects mixed public/private answers, and
// never passes the original hostname to the dialer (avoiding a second lookup).
func PublicTarget(ctx context.Context, target string) (string, error) {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return "", err
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", errors.New("invalid port")
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return "", errors.New("no addresses")
	}
	for _, ip := range ips {
		if !publicIP(ip.IP) || ip.Zone != "" {
			return "", errors.New("non-public target rejected")
		}
	}
	return net.JoinHostPort(ips[0].IP.String(), port), nil
}

func publicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	// IPv6 is conservatively limited to currently allocated global unicast
	// space. This excludes local translation, discard and unallocated ranges.
	if ip.To4() == nil {
		_, global, _ := net.ParseCIDR("2000::/3")
		if !global.Contains(ip) {
			return false
		}
	}
	// Deny special-purpose ranges, including protocol assignments and 6to4.
	// Some globally reachable exceptions inside these ranges are deliberately
	// excluded by the conservative target policy.
	for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

func relay(a, b net.Conn) {
	done := make(chan error, 2)
	copyOne := func(dst, src net.Conn) {
		_, err := io.Copy(dst, src)
		if err == nil {
			if half, ok := dst.(interface{ CloseWrite() error }); ok {
				err = half.CloseWrite()
			} else {
				err = errors.New("half-close unsupported")
			}
		}
		done <- err
	}
	go copyOne(a, b)
	go copyOne(b, a)
	closed := func(c net.Conn) <-chan struct{} {
		if c, ok := c.(interface{ Done() <-chan struct{} }); ok {
			return c.Done()
		}
		return nil
	}
	aClosed, bClosed := closed(a), closed(b)
	for remaining := 2; remaining > 0; {
		select {
		case err := <-done:
			remaining--
			if err == nil {
				continue
			}
		case <-aClosed:
		case <-bClosed:
		}
		// A full tunnel close must wake a copy blocked on the other endpoint,
		// even after its opposite direction has already completed with FIN.
		a.Close()
		b.Close()
		for ; remaining > 0; remaining-- {
			<-done
		}
	}
}
