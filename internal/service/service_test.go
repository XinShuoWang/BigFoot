package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestLimitsIdleAndDrain(t *testing.T) {
	manager := New(Options{MaxConnections: 1, SessionTimeout: time.Second, IdleTimeout: 80 * time.Millisecond})
	handler := manager.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		c := Track(r.Context(), websocket.NetConn(r.Context(), ws, websocket.MessageBinary))
		io.Copy(c, c)
	}))
	s := httptest.NewServer(handler)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(s.URL, "http")
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	other, resp, err := websocket.Dial(ctx, url, nil)
	if other != nil {
		other.CloseNow()
	}
	if err == nil || resp.StatusCode != 503 {
		t.Fatal("capacity not enforced")
	}
	// Traffic in either direction resets the shared idle clock.
	for i := 0; i < 5; i++ {
		time.Sleep(30 * time.Millisecond)
		if err := ws.Write(ctx, websocket.MessageBinary, []byte("x")); err != nil {
			t.Fatal(err)
		}
		if _, _, err := ws.Read(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := ws.Read(ctx); err == nil {
		t.Fatal("idle not closed")
	}
	deadline := time.Now().Add(time.Second)
	for manager.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if manager.Active() != 0 {
		t.Fatal("slot leaked")
	}
	ws2, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws2.CloseNow()
	grace, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	manager.Shutdown(grace)
	if _, _, err := ws2.Read(ctx); err == nil {
		t.Fatal("drain did not close active WS")
	}
	res, err := http.Get(s.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 503 {
		t.Fatal("draining instance ready")
	}
	other, resp, err = websocket.Dial(ctx, url, nil)
	if other != nil {
		other.CloseNow()
	}
	if err == nil || resp.StatusCode != 503 {
		t.Fatal("admitted after drain")
	}
}
