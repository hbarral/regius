package ws

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Event is the wire envelope delivered over WebSocket connections. It
// carries the same {"event", "data"} shape the framework's SSE broker puts
// on the wire, so payloads work over both transports unchanged.
type Event struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// NewEvent builds an Event with data marshaled to JSON.
func NewEvent(name string, data any) (Event, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return Event{}, err
	}
	return Event{Event: name, Data: raw}, nil
}

// ClientInfo describes a connected hub client.
type ClientInfo struct {
	ID          string    `json:"id"`
	RemoteAddr  string    `json:"remote_addr"`
	ConnectedAt time.Time `json:"connected_at"`
	// Meta carries per-connection metadata set by HandlerWithMeta's
	// metaFn at registration time (identity, tenant, …). It is immutable
	// once registration completes, so hooks and concurrent goroutines may
	// read it freely.
	Meta map[string]string `json:"meta,omitempty"`
}

// Hub errors.
var (
	// ErrClientNotFound is returned by Send and Close when no client with
	// the given ID is registered.
	ErrClientNotFound = errors.New("regius/ws: client not found")

	// ErrClientBusy is returned by Send when the client's outbound buffer
	// is full — its write pump has stalled (network backpressure or a
	// dead connection) and the hub policy keeps it registered.
	ErrClientBusy = errors.New("regius/ws: client buffer full")

	// ErrHubFull is returned by the hub's Handler when the client count
	// reached the configured maximum.
	ErrHubFull = errors.New("regius/ws: hub is at maximum client capacity")
)

// Default hub configuration; every value is overridable via an Option.
const (
	defaultClientBuffer   = 16
	defaultHeartbeat      = 30 * time.Second
	defaultPongTimeout    = 60 * time.Second
	defaultMaxMessageSize = 32 * 1024
)

type config struct {
	clientBuffer   int
	heartbeat      time.Duration
	writeTimeout   time.Duration
	pongTimeout    time.Duration
	maxMessageSize int64
	maxClients     int64
	dropSlow       bool
}

func defaultConfig() config {
	return config{
		clientBuffer:   defaultClientBuffer,
		heartbeat:      defaultHeartbeat,
		writeTimeout:   defaultWriteTimeout,
		pongTimeout:    defaultPongTimeout,
		maxMessageSize: defaultMaxMessageSize,
	}
}

// Option configures a Hub; see NewHub.
type Option func(*config)

// WithClientBuffer sets the per-client outbound event buffer size
// (default 16). When the buffer overflows the slow-client policy applies.
func WithClientBuffer(n int) Option {
	return func(c *config) {
		if n >= 0 {
			c.clientBuffer = n
		}
	}
}

// WithHeartbeat sets how often the hub pings connected clients
// (default 30s). A non-positive value disables heartbeats.
func WithHeartbeat(d time.Duration) Option {
	return func(c *config) { c.heartbeat = d }
}

// WithWriteTimeout bounds every server-side write on hub-managed
// connections (default 10s, the package default). A non-positive value
// disables write deadlines.
func WithWriteTimeout(d time.Duration) Option {
	return func(c *config) { c.writeTimeout = d }
}

// WithPongTimeout sets how long the hub waits for proof of liveness (any
// read, or a pong replying to the heartbeat) before dropping a client
// (default 60s). A non-positive value disables the read deadline.
func WithPongTimeout(d time.Duration) Option {
	return func(c *config) { c.pongTimeout = d }
}

// WithMaxMessageSize sets the largest inbound message accepted from a
// client, in bytes (default 32 KiB). Larger messages fail the read and
// close the connection.
func WithMaxMessageSize(n int64) Option {
	return func(c *config) {
		if n > 0 {
			c.maxMessageSize = n
		}
	}
}

// WithMaxClients caps the number of simultaneous clients
// (default 0 = unlimited).
func WithMaxClients(n int64) Option {
	return func(c *config) {
		if n >= 0 {
			c.maxClients = n
		}
	}
}

// WithDropSlow switches the slow-client policy from the default (close the
// client, because silently dropping events breaks ordering) to the SSE
// broker's semantics (drop the event, keep the client).
func WithDropSlow() Option {
	return func(c *config) { c.dropSlow = true }
}

// client is a hub-managed connection: one registered entry plus the two
// pumps that serve it.
type client struct {
	info ClientInfo
	conn *Conn
	send chan Event
	hub  *Hub

	// done is closed exactly once by shutdown; both pumps and the
	// slow-client eviction observe it.
	done chan struct{}

	closeOnce sync.Once
	// reason is written inside closeOnce.Do before close(done); the write
	// pump reads it only after observing done closed, so the channel
	// close provides the happens-before edge.
	reason string
}

// shutdown idempotently signals both pumps to wind down. It never closes
// the send channel: a concurrent Broadcast holding a stale snapshot would
// panic sending on a closed channel; instead Broadcast's select is safe
// against a full or abandoned buffer.
func (c *client) shutdown(reason string) {
	c.closeOnce.Do(func() {
		c.reason = reason
		close(c.done)
	})
}

// Hub broadcasts Events to connected WebSocket clients, mirroring the
// framework's SSE broker: bounded per-client buffers, non-blocking
// broadcast, per-client send.
//
// Each registered client is served by two goroutines: a read pump
// (deadline + pong bookkeeping, inbound messages dispatched to the
// OnMessage hook) and a write pump (drains the outbound buffer, sends
// heartbeat pings, performs the closing handshake).
type Hub struct {
	cfg config

	mu      sync.RWMutex
	clients map[string]*client
	nextID  atomic.Int64

	hooksMu      sync.RWMutex
	onMessage    []*messageHook
	onConnect    []*lifecycleHook
	onDisconnect []*lifecycleHook
}

// lifecycleHook and messageHook are registry entries so listeners can be
// removed by pointer identity (function values are not comparable).
type lifecycleHook struct {
	fn func(info ClientInfo)
}

type messageHook struct {
	fn func(info ClientInfo, ev Event)
}

// NewHub returns a Hub with the given options applied.
func NewHub(opts ...Option) *Hub {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Hub{cfg: cfg, clients: make(map[string]*client)}
}

// OnMessage registers fn as a listener for every inbound client message.
// Listeners run on the client's read pump goroutine, so they must not
// block; forward to a channel or goroutine for slow work. Multiple
// listeners run in registration order; the returned function removes this
// one (removal is optional — dropping the reference simply keeps the
// listener installed for the hub's lifetime).
func (h *Hub) OnMessage(fn func(info ClientInfo, ev Event)) func() {
	entry := &messageHook{fn: fn}
	h.hooksMu.Lock()
	h.onMessage = append(h.onMessage, entry)
	h.hooksMu.Unlock()
	return func() { removeHook(&h.hooksMu, &h.onMessage, entry) }
}

// OnConnect registers fn as a listener invoked when a client registers.
// It fires synchronously from the registering handler's goroutine, before
// the client's pumps start, so the listener's bookkeeping is in place
// before any message can arrive. Multiple listeners run in registration
// order; the returned function removes this one.
func (h *Hub) OnConnect(fn func(info ClientInfo)) func() {
	entry := &lifecycleHook{fn: fn}
	h.hooksMu.Lock()
	h.onConnect = append(h.onConnect, entry)
	h.hooksMu.Unlock()
	return func() { removeHook(&h.hooksMu, &h.onConnect, entry) }
}

// OnDisconnect registers fn as a listener invoked exactly once per client
// when it leaves the registry — client disconnect, oversized message,
// liveness timeout, slow-client eviction, or Hub.Close. It fires after the
// client was removed from the registry but before its connection finishes
// closing. Multiple listeners run in registration order; the returned
// function removes this one.
func (h *Hub) OnDisconnect(fn func(info ClientInfo)) func() {
	entry := &lifecycleHook{fn: fn}
	h.hooksMu.Lock()
	h.onDisconnect = append(h.onDisconnect, entry)
	h.hooksMu.Unlock()
	return func() { removeHook(&h.hooksMu, &h.onDisconnect, entry) }
}

// removeHook drops entry from list by pointer identity (function values
// are not comparable). Generic over the entry type so all three hook lists
// share one implementation.
func removeHook[T any](mu *sync.RWMutex, list *[]*T, entry *T) {
	mu.Lock()
	out := (*list)[:0]
	for _, e := range *list {
		if e != entry {
			out = append(out, e)
		}
	}
	*list = out
	mu.Unlock()
}

// Handler returns the http.HandlerFunc that upgrades requests with u (the
// zero Upgrader when nil) and registers the resulting connection with the
// hub. Mount it wherever the application needs it; on the Regius outer mux
// it bypasses session/CSRF middleware, so authenticated sockets should be
// mounted under the app routes instead.
func (h *Hub) Handler(u *Upgrader) http.HandlerFunc {
	return h.HandlerWithMeta(u, nil)
}

// HandlerWithMeta is Handler with per-connection metadata: metaFn runs
// during the handshake — where session-derived identity is available —
// and its result is stored on ClientInfo.Meta, visible to the
// OnConnect/OnDisconnect hooks, OnMessage, and Clients. A nil metaFn
// behaves exactly like Handler.
func (h *Hub) HandlerWithMeta(u *Upgrader, metaFn func(*http.Request) map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		upgrader := u
		if upgrader == nil {
			upgrader = &Upgrader{}
		}

		h.mu.RLock()
		count := int64(len(h.clients))
		h.mu.RUnlock()
		if h.cfg.maxClients > 0 && count >= h.cfg.maxClients {
			http.Error(w, ErrHubFull.Error(), http.StatusServiceUnavailable)
			return
		}

		conn, err := upgrader.Upgrade(w, r)
		if err != nil {
			// Upgrade already wrote the HTTP error response.
			return
		}

		var meta map[string]string
		if metaFn != nil {
			meta = metaFn(r)
		}

		h.register(conn, meta)
	}
}

// register adds the connection to the hub and starts its pumps. The hub's
// configured write timeout overrides whatever the upgrader carried, so a
// hub's WithWriteTimeout governs its managed connections. The connect
// hook fires after registration but before the pumps start.
func (h *Hub) register(conn *Conn, meta map[string]string) *ClientInfo {
	if h.cfg.writeTimeout > 0 {
		conn.writeTimeout = h.cfg.writeTimeout
	}

	id := strconv.FormatInt(h.nextID.Add(1), 10)

	c := &client{
		info: ClientInfo{
			ID:          id,
			RemoteAddr:  conn.RemoteAddr(),
			ConnectedAt: time.Now(),
			Meta:        meta,
		},
		conn: conn,
		send: make(chan Event, h.cfg.clientBuffer),
		hub:  h,
		done: make(chan struct{}),
	}

	h.mu.Lock()
	h.clients[id] = c
	h.mu.Unlock()

	h.fireConnect(c.info)

	go h.writePump(c)
	go h.readPump(c)

	return &c.info
}

// Broadcast delivers ev to every registered client without blocking: a
// client whose buffer is full hits the slow-client policy (close by
// default; drop with the WithDropSlow option).
func (h *Hub) Broadcast(ev Event) {
	h.mu.RLock()
	snapshot := make([]*client, 0, len(h.clients))
	for _, c := range h.clients {
		snapshot = append(snapshot, c)
	}
	h.mu.RUnlock()

	for _, c := range snapshot {
		select {
		case c.send <- ev:
		default:
			h.slowClient(c)
		}
	}
}

// Send delivers ev to the client with the given ID.
func (h *Hub) Send(clientID string, ev Event) error {
	h.mu.RLock()
	c, ok := h.clients[clientID]
	h.mu.RUnlock()
	if !ok {
		return ErrClientNotFound
	}

	select {
	case c.send <- ev:
		return nil
	default:
		return ErrClientBusy
	}
}

// Close unregisters the client and closes its connection with the given
// reason, performing the closing handshake. The disconnect hook fires
// like any other exit path.
func (h *Hub) Close(clientID string, reason string) error {
	h.mu.RLock()
	c, ok := h.clients[clientID]
	h.mu.RUnlock()
	if !ok {
		return ErrClientNotFound
	}

	if !h.unregister(c) {
		// A concurrent exit path won the race between the check above
		// and the removal.
		return ErrClientNotFound
	}

	c.shutdown(reason)
	return nil
}

// Clients returns a snapshot of the connected clients, oldest first.
func (h *Hub) Clients() []ClientInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	infos := make([]ClientInfo, 0, len(h.clients))
	for _, c := range h.clients {
		infos = append(infos, c.info)
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ConnectedAt.Before(infos[j].ConnectedAt)
	})
	return infos
}

// slowClient applies the configured policy to a client whose outbound
// buffer is full.
func (h *Hub) slowClient(c *client) {
	if h.cfg.dropSlow {
		return
	}
	h.remove(c, "slow client: outbound buffer full")
}

// remove unregisters the client (if still present) and signals its pumps.
func (h *Hub) remove(c *client, reason string) {
	h.unregister(c)
	c.shutdown(reason)
}

// unregister removes the client from the registry when it is still the
// registered entry, firing the disconnect hook exactly once per client
// (read-pump exit, write-pump failure, slow-client eviction, and Close all
// funnel through here; double removals are no-ops). It reports whether
// this call was the one that removed the client.
func (h *Hub) unregister(c *client) bool {
	h.mu.Lock()
	cur, ok := h.clients[c.info.ID]
	removed := ok && cur == c
	if removed {
		delete(h.clients, c.info.ID)
	}
	h.mu.Unlock()

	if removed {
		h.fireDisconnect(c.info)
	}
	return removed
}

// fireConnect invokes the connect listeners. The list is copied before
// invoking so a listener may (un)register hooks without deadlocking on
// hooksMu.
func (h *Hub) fireConnect(info ClientInfo) {
	h.hooksMu.RLock()
	listeners := append([]*lifecycleHook(nil), h.onConnect...)
	h.hooksMu.RUnlock()
	for _, entry := range listeners {
		entry.fn(info)
	}
}

// fireDisconnect invokes the disconnect listeners; see fireConnect for the
// copy-then-invoke rationale.
func (h *Hub) fireDisconnect(info ClientInfo) {
	h.hooksMu.RLock()
	listeners := append([]*lifecycleHook(nil), h.onDisconnect...)
	h.hooksMu.RUnlock()
	for _, entry := range listeners {
		entry.fn(info)
	}
}

// fireMessage invokes the message listeners; see fireConnect for the
// copy-then-invoke rationale.
func (h *Hub) fireMessage(info ClientInfo, ev Event) {
	h.hooksMu.RLock()
	listeners := append([]*messageHook(nil), h.onMessage...)
	h.hooksMu.RUnlock()
	for _, entry := range listeners {
		entry.fn(info, ev)
	}
}

// readPump drains inbound messages until the connection dies, the liveness
// deadline expires, or the hub shuts the client down.
func (h *Hub) readPump(c *client) {
	defer h.remove(c, "client disconnected")

	if h.cfg.maxMessageSize > 0 {
		c.conn.SetReadLimit(h.cfg.maxMessageSize)
	}
	h.extendReadDeadline(c)
	c.conn.SetPongHandler(func(string) error {
		return h.extendReadDeadline(c)
	})

	for {
		select {
		case <-c.done:
			return
		default:
		}

		var ev Event
		if err := c.conn.ReadJSON(&ev); err != nil {
			return
		}

		h.fireMessage(c.info, ev)
	}
}

// extendReadDeadline pushes the liveness deadline out by pongTimeout.
func (h *Hub) extendReadDeadline(c *client) error {
	if h.cfg.pongTimeout <= 0 {
		return nil
	}
	return c.conn.SetReadDeadline(time.Now().Add(h.cfg.pongTimeout))
}

// writePump drains the outbound buffer and sends heartbeat pings until the
// hub shuts the client down or a write fails; on shutdown it performs the
// closing handshake with the stored reason.
func (h *Hub) writePump(c *client) {
	var ticker *time.Ticker
	var tickerC <-chan time.Time
	if h.cfg.heartbeat > 0 {
		ticker = time.NewTicker(h.cfg.heartbeat)
		defer ticker.Stop()
		tickerC = ticker.C
	}

	for {
		select {
		case ev := <-c.send:
			if err := c.conn.WriteJSON(ev); err != nil {
				h.remove(c, "write failed")
				return
			}
		case <-tickerC:
			if err := c.conn.Ping(); err != nil {
				h.remove(c, "ping failed")
				return
			}
		case <-c.done:
			// Closing handshake with the shutdown reason; the reason
			// field was written before done was closed, so the channel
			// close provides the happens-before edge.
			c.conn.Close(c.reason)
			return
		}
	}
}
