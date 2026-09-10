package ws

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// dialHub starts a test server for hub.Handler and dials n clients.
func dialHub(t *testing.T, hub *Hub, upgrader *Upgrader, n int) []*websocket.Conn {
	t.Helper()

	server := httptest.NewServer(hub.Handler(upgrader))
	t.Cleanup(server.Close)

	conns := make([]*websocket.Conn, 0, n)
	for i := 0; i < n; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
		if err != nil {
			for _, c := range conns {
				c.Close()
			}
			t.Fatalf("client %d dial error = %v", i, err)
		}
		conns = append(conns, conn)
	}

	// The handshake response returns before register() runs in the
	// handler; give the hub a moment so Clients() is populated.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(hub.Clients()) == n {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := len(hub.Clients()); got != n {
		for _, c := range conns {
			c.Close()
		}
		t.Fatalf("hub registered %d clients, want %d", got, n)
	}

	t.Cleanup(func() {
		for _, c := range conns {
			c.Close()
		}
	})

	return conns
}

func readEvent(t *testing.T, conn *websocket.Conn) Event {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	var ev Event
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	return ev
}

func TestNewEvent(t *testing.T) {
	ev, err := NewEvent("user.joined", map[string]string{"name": "ada"})
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}
	if ev.Event != "user.joined" {
		t.Fatalf("event = %q, want %q", ev.Event, "user.joined")
	}
	if string(ev.Data) != `{"name":"ada"}` {
		t.Fatalf("data = %s, want {\"name\":\"ada\"}", ev.Data)
	}

	if _, err := NewEvent("bad", make(chan int)); err == nil {
		t.Fatal("NewEvent() with unmarshalable data should fail")
	}
}

func TestHub_BroadcastAndSend(t *testing.T) {
	hub := NewHub()
	conns := dialHub(t, hub, nil, 2)

	ev, err := NewEvent("ping", "pong")
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}
	hub.Broadcast(ev)

	for i, conn := range conns {
		got := readEvent(t, conn)
		if got.Event != "ping" || string(got.Data) != `"pong"` {
			t.Fatalf("client %d received %+v, want ping/pong", i, got)
		}
	}

	clients := hub.Clients()
	if len(clients) != 2 {
		t.Fatalf("Clients() = %d entries, want 2", len(clients))
	}

	if err := hub.Send(clients[0].ID, ev); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	got := readEvent(t, conns[0])
	if got.Event != "ping" {
		t.Fatalf("targeted client received %+v, want ping", got)
	}

	if err := hub.Send("no-such-client", ev); !errors.Is(err, ErrClientNotFound) {
		t.Fatalf("Send() error = %v, want ErrClientNotFound", err)
	}
}

func TestHub_OnMessage(t *testing.T) {
	hub := NewHub()

	received := make(chan ClientEvent, 1)
	hub.OnMessage(func(info ClientInfo, ev Event) {
		select {
		case received <- ClientEvent{Info: info, Event: ev}:
		default:
		}
	})

	conns := dialHub(t, hub, nil, 1)

	if err := conns[0].WriteJSON(Event{Event: "chat", Data: []byte(`"hi"`)}); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}

	select {
	case got := <-received:
		if got.Event.Event != "chat" || string(got.Event.Data) != `"hi"` {
			t.Fatalf("OnMessage received %+v, want chat/hi", got.Event)
		}
		if got.Info.ID == "" {
			t.Fatal("OnMessage client info has empty ID")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnMessage callback")
	}
}

type ClientEvent struct {
	Info  ClientInfo
	Event Event
}

func TestHub_CloseHandshake(t *testing.T) {
	hub := NewHub()
	conns := dialHub(t, hub, nil, 1)

	clientID := hub.Clients()[0].ID
	if err := hub.Close(clientID, "server shutdown"); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := conns[0].SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	_, _, err := conns[0].ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		t.Fatalf("ReadMessage() error = %v, want close error", err)
	}
	if closeErr.Code != websocket.CloseNormalClosure {
		t.Fatalf("close code = %d, want %d", closeErr.Code, websocket.CloseNormalClosure)
	}
	if closeErr.Text != "server shutdown" {
		t.Fatalf("close text = %q, want %q", closeErr.Text, "server shutdown")
	}

	if err := hub.Close(clientID, "again"); !errors.Is(err, ErrClientNotFound) {
		t.Fatalf("Close() error = %v, want ErrClientNotFound", err)
	}
}

func TestHub_Heartbeat(t *testing.T) {
	hub := NewHub(WithHeartbeat(25 * time.Millisecond))
	conns := dialHub(t, hub, nil, 1)

	var pings atomic.Int64
	conns[0].SetPingHandler(func(appData string) error {
		pings.Add(1)
		return conns[0].WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
	})

	// Control frames are only processed while reading: drain the
	// connection in the background while the hub keeps pinging. Data
	// events surface on the events channel.
	events := make(chan Event, 8)
	go func() {
		for {
			var ev Event
			if err := conns[0].ReadJSON(&ev); err != nil {
				return
			}
			select {
			case events <- ev:
			default:
			}
		}
	}()

	// The connection stays alive purely through heartbeats: no data
	// message is ever sent, and the hub keeps pinging.
	time.Sleep(300 * time.Millisecond)
	if got := pings.Load(); got < 3 {
		t.Fatalf("received %d pings in 300ms with 25ms heartbeat, want >= 3", got)
	}

	// The client is still registered and reachable.
	ev, _ := NewEvent("still-here", nil)
	hub.Broadcast(ev)
	select {
	case got := <-events:
		if got.Event != "still-here" {
			t.Fatalf("received %+v, want still-here", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for broadcast after heartbeat window")
	}
}

func TestHub_MaxMessageSize(t *testing.T) {
	hub := NewHub(WithMaxMessageSize(64))
	conns := dialHub(t, hub, nil, 1)

	big := make([]byte, 512)
	if err := conns[0].WriteMessage(websocket.TextMessage, big); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}

	// The oversized read fails, the hub tears the client down, and the
	// client observes the closing handshake or a dropped connection.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := conns[0].SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			t.Fatalf("SetReadDeadline() error = %v", err)
		}
		if _, _, err := conns[0].ReadMessage(); err != nil {
			break // connection is gone: expected
		}
		if len(hub.Clients()) == 0 {
			break
		}
	}
	if got := len(hub.Clients()); got != 0 {
		t.Fatalf("hub still has %d clients after oversized message, want 0", got)
	}
}

func TestHub_MaxClients(t *testing.T) {
	hub := NewHub(WithMaxClients(1))
	conns := dialHub(t, hub, nil, 1)

	server := httptest.NewServer(hub.Handler(nil))
	defer server.Close()

	_, resp, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err == nil {
		t.Fatal("second dial succeeded at capacity, want 503")
	}
	if resp == nil {
		t.Fatalf("dial error without response: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	// The first client is unaffected.
	ev, _ := NewEvent("ping", nil)
	hub.Broadcast(ev)
	if got := readEvent(t, conns[0]); got.Event != "ping" {
		t.Fatalf("first client received %+v, want ping", got)
	}
}

func TestHub_SlowClient_ClosedByDefault(t *testing.T) {
	hub := NewHub(WithClientBuffer(1))

	// A white-box client with a full buffer and no pumps: the default
	// policy must unregister it and signal shutdown.
	c := &client{
		info: ClientInfo{ID: "1", ConnectedAt: time.Now()},
		conn: &Conn{},
		send: make(chan Event, 1),
		hub:  hub,
		done: make(chan struct{}),
	}
	hub.clients["1"] = c
	c.send <- Event{Event: "filler"}

	hub.Broadcast(Event{Event: "overflow"})

	select {
	case <-c.done:
		// expected: slow client evicted
	case <-time.After(2 * time.Second):
		t.Fatal("slow client was not shut down by the default policy")
	}
	if _, ok := hub.clients["1"]; ok {
		t.Fatal("slow client still registered after eviction")
	}
	if c.reason != "slow client: outbound buffer full" {
		t.Fatalf("shutdown reason = %q, want slow-client reason", c.reason)
	}
}

func TestHub_SlowClient_DropPolicy(t *testing.T) {
	hub := NewHub(WithClientBuffer(1), WithDropSlow())

	c := &client{
		info: ClientInfo{ID: "1", ConnectedAt: time.Now()},
		conn: &Conn{},
		send: make(chan Event, 1),
		hub:  hub,
		done: make(chan struct{}),
	}
	hub.clients["1"] = c
	c.send <- Event{Event: "filler"}

	hub.Broadcast(Event{Event: "overflow"})

	select {
	case <-c.done:
		t.Fatal("client shut down under drop policy, want it kept")
	default:
	}
	if _, ok := hub.clients["1"]; !ok {
		t.Fatal("client unregistered under drop policy, want it kept")
	}

	// The overflow event was dropped, not queued: the buffer still holds
	// only the filler.
	select {
	case ev := <-c.send:
		if ev.Event != "filler" {
			t.Fatalf("buffer head = %q, want filler (overflow should be dropped)", ev.Event)
		}
	default:
		t.Fatal("buffer empty, want the filler event preserved")
	}
}

func TestHub_ClientDisconnectUnregisters(t *testing.T) {
	hub := NewHub()
	conns := dialHub(t, hub, nil, 1)

	conns[0].Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(hub.Clients()) == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := len(hub.Clients()); got != 0 {
		t.Fatalf("hub has %d clients after disconnect, want 0", got)
	}
}

func TestHub_ConcurrentChurn(t *testing.T) {
	hub := NewHub(WithClientBuffer(4))

	server := httptest.NewServer(hub.Handler(nil))
	defer server.Close()
	url := wsURL(server.URL)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Broadcasters keep firing while clients connect and disconnect.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev, _ := NewEvent("tick", nil)
			for {
				select {
				case <-stop:
					return
				default:
					hub.Broadcast(ev)
					time.Sleep(2 * time.Millisecond)
				}
			}
		}()
	}

	// Churning clients: dial, read a little, drop without a closing
	// handshake (raw TCP close is the hostile path).
	var churnWG sync.WaitGroup
	for i := 0; i < 16; i++ {
		churnWG.Add(1)
		go func() {
			defer churnWG.Done()
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				return
			}
			conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			for {
				var ev Event
				if err := conn.ReadJSON(&ev); err != nil {
					break
				}
			}
			conn.UnderlyingConn().Close()
		}()
	}
	churnWG.Wait()
	close(stop)
	wg.Wait()

	// Every churned client must eventually be reaped.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(hub.Clients()) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(hub.Clients()); got != 0 {
		t.Fatalf("hub has %d clients after churn, want 0", got)
	}
}

// hookRecorder collects connect/disconnect hook events with their Meta,
// for the hook tests below.
type hookRecorder struct {
	mu          sync.Mutex
	connects    []ClientInfo
	disconnects []ClientInfo
}

func (rec *hookRecorder) onConnect(info ClientInfo) {
	rec.mu.Lock()
	rec.connects = append(rec.connects, info)
	rec.mu.Unlock()
}

func (rec *hookRecorder) onDisconnect(info ClientInfo) {
	rec.mu.Lock()
	rec.disconnects = append(rec.disconnects, info)
	rec.mu.Unlock()
}

func (rec *hookRecorder) waitFor(t *testing.T, n int, which string) []ClientInfo {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec.mu.Lock()
		var got int
		if which == "connect" {
			got = len(rec.connects)
		} else {
			got = len(rec.disconnects)
		}
		rec.mu.Unlock()
		if got >= n {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			if which == "connect" {
				return rec.connects
			}
			return rec.disconnects
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d %s hooks", n, which)
	return nil
}

func TestHub_MetaRoundTrip(t *testing.T) {
	hub := NewHub()
	handler := hub.HandlerWithMeta(nil, func(r *http.Request) map[string]string {
		return map[string]string{
			"notify.user": r.Header.Get("X-Test-User"),
			"static":      "yes",
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), http.Header{
		"X-Test-User": []string{"42"},
	})
	if err != nil {
		t.Fatalf("dial error = %v", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if clients := hub.Clients(); len(clients) == 1 {
			if clients[0].Meta["notify.user"] != "42" || clients[0].Meta["static"] != "yes" {
				t.Fatalf("meta = %v, want notify.user=42 static=yes", clients[0].Meta)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("client never registered")
}

func TestHub_ConnectDisconnectHooks(t *testing.T) {
	hub := NewHub()
	rec := &hookRecorder{}
	hub.OnConnect(rec.onConnect)
	hub.OnDisconnect(rec.onDisconnect)

	conns := dialHub(t, hub, nil, 1)

	connects := rec.waitFor(t, 1, "connect")
	if connects[0].ID == "" || connects[0].RemoteAddr == "" {
		t.Fatalf("connect hook got incomplete info: %+v", connects[0])
	}

	// Exit path 1: the client drops the connection.
	conns[0].Close()
	disconnects := rec.waitFor(t, 1, "disconnect")
	if disconnects[0].ID != connects[0].ID {
		t.Fatalf("disconnect ID = %q, want %q", disconnects[0].ID, connects[0].ID)
	}
}

func TestHub_DisconnectHook_HubClose(t *testing.T) {
	hub := NewHub()
	rec := &hookRecorder{}
	hub.OnDisconnect(rec.onDisconnect)

	dialHub(t, hub, nil, 1)
	rec.waitFor(t, 0, "connect") // no connect hook installed

	clientID := hub.Clients()[0].ID
	if err := hub.Close(clientID, "bye"); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	disconnects := rec.waitFor(t, 1, "disconnect")
	if disconnects[0].ID != clientID {
		t.Fatalf("disconnect ID = %q, want %q", disconnects[0].ID, clientID)
	}

	// A second Close on the already-removed client must not fire the hook
	// again (exactly-once semantics): give a racing hook a moment, then
	// assert the count is still one.
	if err := hub.Close(clientID, "again"); !errors.Is(err, ErrClientNotFound) {
		t.Fatalf("second Close() error = %v, want ErrClientNotFound", err)
	}
	time.Sleep(100 * time.Millisecond)
	rec.mu.Lock()
	got := len(rec.disconnects)
	rec.mu.Unlock()
	if got != 1 {
		t.Fatalf("disconnect hook fired %d times, want exactly 1", got)
	}
}

func TestHub_DisconnectHook_OversizedMessage(t *testing.T) {
	hub := NewHub(WithMaxMessageSize(64))
	rec := &hookRecorder{}
	hub.OnDisconnect(rec.onDisconnect)

	conns := dialHub(t, hub, nil, 1)

	big := make([]byte, 512)
	if err := conns[0].WriteMessage(websocket.TextMessage, big); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}

	rec.waitFor(t, 1, "disconnect")
}

func TestHub_DisconnectHook_SlowClientEviction(t *testing.T) {
	hub := NewHub(WithClientBuffer(1))
	rec := &hookRecorder{}
	hub.OnDisconnect(rec.onDisconnect)

	c := &client{
		info: ClientInfo{ID: "1", ConnectedAt: time.Now()},
		conn: &Conn{},
		send: make(chan Event, 1),
		hub:  hub,
		done: make(chan struct{}),
	}
	hub.clients["1"] = c
	c.send <- Event{Event: "filler"}

	hub.Broadcast(Event{Event: "overflow"})

	disconnects := rec.waitFor(t, 1, "disconnect")
	if disconnects[0].ID != "1" {
		t.Fatalf("disconnect ID = %q, want 1", disconnects[0].ID)
	}
}

func TestHub_HooksCanCallHub(t *testing.T) {
	hub := NewHub()

	// Hooks firing while the registry lock was just released must be able
	// to call back into the hub without deadlocking.
	hub.OnConnect(func(info ClientInfo) {
		_ = hub.Clients()
		_ = hub.Send(info.ID, Event{Event: "welcome"})
	})
	hub.OnDisconnect(func(info ClientInfo) {
		_ = hub.Clients()
	})

	conns := dialHub(t, hub, nil, 1)
	ev := readEvent(t, conns[0])
	if ev.Event != "welcome" {
		t.Fatalf("received %+v, want welcome (sent from inside the connect hook)", ev)
	}
	conns[0].Close()
}

func TestHub_OnConnect_Replaced(t *testing.T) {
	hub := NewHub()
	rec := &hookRecorder{}
	hub.OnConnect(rec.onConnect)
	rec2 := &hookRecorder{}
	hub.OnConnect(rec2.onConnect) // replaces

	dialHub(t, hub, nil, 1)

	rec2.waitFor(t, 1, "connect")
	rec.mu.Lock()
	got := len(rec.connects)
	rec.mu.Unlock()
	if got != 0 {
		t.Fatalf("replaced hook still fired %d times", got)
	}
}
