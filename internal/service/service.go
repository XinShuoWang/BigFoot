// Package service bounds and drains long-lived HTTP upgrade sessions.
package service

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Options struct {
	MaxConnections int
	SessionTimeout time.Duration
	IdleTimeout    time.Duration
	Logger         *slog.Logger
}

type Manager struct {
	options  Options
	mu       sync.Mutex
	draining bool
	next     uint64
	sessions map[uint64]context.CancelFunc
	empty    chan struct{}
}

func New(options Options) *Manager {
	if options.MaxConnections <= 0 {
		options.MaxConnections = 128
	}
	if options.SessionTimeout <= 0 {
		options.SessionTimeout = 15 * time.Minute
	}
	if options.IdleTimeout <= 0 {
		options.IdleTimeout = 2 * time.Minute
	}
	empty := make(chan struct{})
	close(empty)
	return &Manager{options: options, sessions: make(map[uint64]context.CancelFunc), empty: empty}
}

func (m *Manager) Active() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.sessions) }

func (m *Manager) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(200)
			return
		}
		m.mu.Lock()
		draining := m.draining
		m.mu.Unlock()
		if r.URL.Path == "/readyz" {
			if draining {
				http.Error(w, "draining", 503)
			} else {
				w.WriteHeader(200)
			}
			return
		}
		// Ordinary requests do not consume tunnel capacity. Local CONNECT does.
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") && r.Method != "CONNECT" {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), m.options.SessionTimeout)
		defer cancel()
		m.mu.Lock()
		if m.draining || len(m.sessions) >= m.options.MaxConnections {
			m.mu.Unlock()
			w.Header().Set("Retry-After", "1")
			http.Error(w, "service busy", 503)
			return
		}
		if len(m.sessions) == 0 {
			m.empty = make(chan struct{})
		}
		m.next++
		id := m.next
		m.sessions[id] = cancel
		m.mu.Unlock()
		started := time.Now()
		defer func() {
			m.mu.Lock()
			delete(m.sessions, id)
			if len(m.sessions) == 0 {
				close(m.empty)
			}
			m.mu.Unlock()
			if m.options.Logger != nil {
				m.options.Logger.Info("session closed", "duration_ms", time.Since(started).Milliseconds())
			}
		}()
		last := &atomic.Int64{}
		last.Store(time.Now().UnixNano())
		ctx = context.WithValue(ctx, activityKey{}, last)
		idleDone := make(chan struct{})
		go func() {
			defer close(idleDone)
			timer := time.NewTimer(m.options.IdleTimeout)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
					remaining := m.options.IdleTimeout - time.Since(time.Unix(0, last.Load()))
					if remaining <= 0 {
						cancel()
						return
					}
					timer.Reset(remaining)
				}
			}
		}()
		defer func() { cancel(); <-idleDone }()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Shutdown stops admission, allows active sessions to finish until ctx expires,
// then cancels every remaining session (including HTTP-hijacked WebSockets).
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	m.draining = true
	empty := m.empty
	m.mu.Unlock()
	select {
	case <-empty:
		return nil
	case <-ctx.Done():
	}
	m.mu.Lock()
	for _, cancel := range m.sessions {
		cancel()
	}
	m.mu.Unlock()
	return ctx.Err()
}

type activityKey struct{}
type activityConn struct {
	net.Conn
	last *atomic.Int64
}

func Track(ctx context.Context, c net.Conn) net.Conn {
	last, _ := ctx.Value(activityKey{}).(*atomic.Int64)
	if last == nil {
		return c
	}
	return &activityConn{Conn: c, last: last}
}
func (c *activityConn) Read(p []byte) (int, error) {
	n, e := c.Conn.Read(p)
	if n > 0 {
		c.last.Store(time.Now().UnixNano())
	}
	return n, e
}
func (c *activityConn) Write(p []byte) (int, error) {
	n, e := c.Conn.Write(p)
	if n > 0 {
		c.last.Store(time.Now().UnixNano())
	}
	return n, e
}
func (c *activityConn) CloseWrite() error {
	if h, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return h.CloseWrite()
	}
	return errors.New("half-close unsupported")
}
func (c *activityConn) Done() <-chan struct{} {
	if h, ok := c.Conn.(interface{ Done() <-chan struct{} }); ok {
		return h.Done()
	}
	return nil
}
