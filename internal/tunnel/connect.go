package tunnel

import (
	"bufio"
	"context"
	"io"
	"local/tunnel-prototype/internal/service"
	"net"
	"net/http"
)

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
func (c *bufferedConn) CloseWrite() error {
	return c.Conn.(interface{ CloseWrite() error }).CloseWrite()
}

// ConnectHandler is a local test adapter. The CLI enforces loopback-only bind.
// This handler is NOT the production, authenticated WSS client ingress.
func ConnectHandler(dial func(context.Context, string) (net.Conn, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" {
			http.Error(w, "use HTTP CONNECT", http.StatusMethodNotAllowed)
			return
		}
		if _, _, err := net.SplitHostPort(r.Host); err != nil {
			http.Error(w, "invalid target", http.StatusBadRequest)
			return
		}
		remote, err := dial(r.Context(), r.Host)
		if err != nil {
			http.Error(w, "tunnel unavailable", http.StatusBadGateway)
			return
		}
		defer remote.Close()
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack unavailable", http.StatusInternalServerError)
			return
		}
		local, buf, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer local.Close()
		if _, err = io.WriteString(buf, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		if err = buf.Flush(); err != nil {
			return
		}
		relay(service.Track(r.Context(), &bufferedConn{Conn: local, reader: buf.Reader}), remote)
	})
}
