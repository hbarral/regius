package ws

import (
	"time"

	"github.com/gorilla/websocket"
)

// Conn is a WebSocket connection upgraded by [Upgrader]. It wraps
// gorilla's connection so gorilla types never leak into application code.
//
// A Conn obtained directly from an Upgrader is driven entirely by the
// application (read/write loops, deadlines). A Conn registered with a
// [Hub] is managed by the hub's per-client pumps instead.
//
// All writes are bounded by the upgrader's WriteTimeout; reads are bounded
// by whatever read deadline the driver (application or hub) installs.
type Conn struct {
	ws           *websocket.Conn
	writeTimeout time.Duration
}

// ReadJSON reads the next message and unmarshals it into v. It returns an
// error when the peer sends a message larger than the read limit installed
// via SetReadLimit, when the read deadline expires, or when the connection
// is closed.
func (c *Conn) ReadJSON(v any) error {
	return c.ws.ReadJSON(v)
}

// WriteJSON marshals v and writes it as a single message within the
// connection's write timeout.
func (c *Conn) WriteJSON(v any) error {
	if err := c.setWriteDeadline(); err != nil {
		return err
	}
	return c.ws.WriteJSON(v)
}

// WriteEvent writes ev using the {"event", "data"} envelope, mirroring what
// the hub's write pump delivers to subscribed clients.
func (c *Conn) WriteEvent(ev Event) error {
	return c.WriteJSON(ev)
}

// Ping sends a control ping within the connection's write timeout.
func (c *Conn) Ping() error {
	return c.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(c.writeTimeout))
}

// SetReadLimit sets the maximum message size in bytes accepted on this
// connection. Reads of larger messages fail and the connection is torn
// down by the driver. Zero means the gorilla default (no limit).
func (c *Conn) SetReadLimit(limit int64) {
	c.ws.SetReadLimit(limit)
}

// SetReadDeadline sets the read deadline; a zero time disables it.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.ws.SetReadDeadline(t)
}

// SetPongHandler installs the callback invoked on received pong control
// frames. The hub uses it to extend its read deadline (liveness proof).
func (c *Conn) SetPongHandler(h func(appData string) error) {
	c.ws.SetPongHandler(h)
}

// RemoteAddr returns the peer's network address.
func (c *Conn) RemoteAddr() string {
	return c.ws.RemoteAddr().String()
}

// Close performs the closing handshake (close frame with the given reason,
// normal closure code) and then closes the underlying network connection.
func (c *Conn) Close(reason string) error {
	err := c.ws.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason),
		time.Now().Add(c.writeTimeout),
	)
	closeErr := c.ws.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (c *Conn) setWriteDeadline() error {
	if c.writeTimeout <= 0 {
		return nil
	}
	return c.ws.SetWriteDeadline(time.Now().Add(c.writeTimeout))
}
