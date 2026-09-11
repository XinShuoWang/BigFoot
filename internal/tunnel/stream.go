package tunnel

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
)

// Conn carries bounded DATA and FIN frames INSIDE TLS. FIN closes one direction
// without terminating the opposite direction or the underlying WebSocket.
type Conn struct {
	net.Conn
	readMu    sync.Mutex
	writeMu   sync.Mutex
	remaining uint32
	readEOF   bool
	writeEOF  bool
	closeFn   func() error
	closeOnce sync.Once
	closeErr  error
	doneOnce  sync.Once
	done      chan struct{}
}

const maxFrame = 32 * 1024

func writeAll(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		p = p[n:]
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func (c *Conn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	if c.readEOF {
		return 0, io.EOF
	}
	if c.remaining == 0 {
		var h [5]byte
		if _, err := io.ReadFull(c.Conn, h[:]); err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return 0, err
		}
		n := binary.BigEndian.Uint32(h[1:])
		if h[0] == 1 && n == 0 {
			c.readEOF = true
			// FIN ends application data, not transport lifetime. Keep a reader
			// active so a later disconnect or illegal post-FIN data closes the
			// opposite TCP endpoint even if it never sends another byte.
			go func() {
				var extra [1]byte
				io.ReadFull(c.Conn, extra[:])
				c.Close()
			}()
			return 0, io.EOF
		}
		if h[0] != 0 || n == 0 || n > maxFrame {
			return 0, errors.New("invalid data frame")
		}
		c.remaining = n
	}
	if uint32(len(p)) > c.remaining {
		p = p[:c.remaining]
	}
	n, err := c.Conn.Read(p)
	c.remaining -= uint32(n)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}
func (c *Conn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.writeEOF {
		return 0, net.ErrClosed
	}
	total := 0
	for len(p) > 0 {
		n := len(p)
		if n > maxFrame {
			n = maxFrame
		}
		var h [5]byte
		binary.BigEndian.PutUint32(h[1:], uint32(n))
		if err := writeAll(c.Conn, h[:]); err != nil {
			return total, err
		}
		if err := writeAll(c.Conn, p[:n]); err != nil {
			return total, err
		}
		total += n
		p = p[n:]
	}
	return total, nil
}
func (c *Conn) CloseWrite() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.writeEOF {
		return nil
	}
	c.writeEOF = true
	return writeAll(c.Conn, []byte{1, 0, 0, 0, 0})
}
func (c *Conn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed())
		if c.closeFn != nil {
			c.closeErr = c.closeFn()
		} else {
			c.closeErr = c.Conn.Close()
		}
	})
	return c.closeErr
}

func (c *Conn) closed() chan struct{} {
	c.doneOnce.Do(func() { c.done = make(chan struct{}) })
	return c.done
}

// Done signals full connection closure, not a directional FIN.
func (c *Conn) Done() <-chan struct{} { return c.closed() }
