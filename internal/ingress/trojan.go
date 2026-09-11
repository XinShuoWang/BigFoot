// Package ingress accepts authenticated Trojan TCP requests over WebSocket.
package ingress

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"local/tunnel-prototype/internal/service"
)

// Server accepts ordinary Trojan over WebSocket, without mux or early data.
// TLS must be provided by the HTTP server or a trusted platform gateway.
type Server struct {
	Passwords        []string
	Path             string
	HandshakeTimeout time.Duration
	Dial             func(context.Context, string) (net.Conn, error)
}

func readRequest(r io.Reader, passwords []string) (string, error) {
	var auth [58]byte
	if _, err := io.ReadFull(r, auth[:]); err != nil {
		return "", err
	}
	valid := 0
	for _, password := range passwords {
		sum := sha256.Sum224([]byte(password))
		encoded := hex.EncodeToString(sum[:])
		valid |= subtle.ConstantTimeCompare(auth[:56], []byte(encoded))
	}
	if valid != 1 || string(auth[56:]) != "\r\n" {
		return "", errors.New("authentication failed")
	}
	var cmd [2]byte
	if _, err := io.ReadFull(r, cmd[:]); err != nil {
		return "", err
	}
	if cmd[0] != 1 {
		return "", errors.New("only TCP CONNECT is supported")
	}
	var host string
	switch cmd[1] {
	case 1, 4:
		n := 4
		if cmd[1] == 4 {
			n = 16
		}
		ip := make([]byte, n)
		if _, err := io.ReadFull(r, ip); err != nil {
			return "", err
		}
		host = net.IP(ip).String()
	case 3:
		var size [1]byte
		if _, err := io.ReadFull(r, size[:]); err != nil {
			return "", err
		}
		if size[0] == 0 {
			return "", errors.New("empty hostname")
		}
		p := make([]byte, int(size[0]))
		if _, err := io.ReadFull(r, p); err != nil {
			return "", err
		}
		host = string(p)
		// Domain ATYP must not smuggle ports, zones or control bytes to the resolver.
		for _, ch := range p {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '-') {
				return "", errors.New("invalid hostname")
			}
		}
	default:
		return "", errors.New("invalid address type")
	}
	var tail [4]byte
	if _, err := io.ReadFull(r, tail[:]); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(tail[:2])
	if port == 0 || string(tail[2:]) != "\r\n" {
		return "", errors.New("invalid request tail")
	}
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

func (s *Server) Handler() http.Handler {
	path := s.Path
	if path == "" {
		path = "/proxy"
	}
	timeout := s.HandshakeTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, "<!doctype html><html><title>Welcome</title><body><h1>Welcome</h1></body></html>")
			return
		}
		if len(s.Passwords) == 0 || s.Dial == nil {
			http.Error(w, "service unavailable", 503)
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
		if err != nil {
			return
		}
		defer ws.CloseNow()
		raw := service.Track(ctx, websocket.NetConn(ctx, ws, websocket.MessageBinary))
		ws.SetReadLimit(1 << 20)
		stop := context.AfterFunc(ctx, func() { ws.CloseNow() })
		defer stop()
		raw.SetReadDeadline(time.Now().Add(timeout))
		target, err := readRequest(raw, s.Passwords)
		if err != nil {
			return
		}
		raw.SetReadDeadline(time.Time{})
		peer, err := s.Dial(ctx, target)
		if err != nil {
			return
		}
		defer peer.Close()
		stopPeer := context.AfterFunc(ctx, func() { peer.Close() })
		defer stopPeer()
		// A WebSocket close is a full close, not TCP FIN. Do not invent an
		// application FIN that standard Trojan/WebSocket clients cannot send.
		done := make(chan struct{}, 2)
		go func() { io.Copy(peer, raw); done <- struct{}{} }()
		go func() { io.Copy(raw, peer); done <- struct{}{} }()
		<-done
		cancel()
		ws.CloseNow()
		peer.Close()
		<-done
	})
}
