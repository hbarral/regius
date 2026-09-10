package regius

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/hbarral/regius/ws"
)

// newTestNotifier builds a notifier over a fresh broker and hub, with an
// identity function reading the X-Notify-User header, and serves both
// handlers from one test server.
func newTestNotifier(t *testing.T) (*Notifier, *httptest.Server) {
	t.Helper()

	broker := NewSSEBroker()
	hub := ws.NewHub()
	notifier := NewNotifier(broker, hub, func(r *http.Request) string {
		return r.Header.Get("X-Notify-User")
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/sse/notify", notifier.SSEHandler())
	mux.HandleFunc("/ws/notify", notifier.WSHandler(nil))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return notifier, server
}

// notifyWSClient dials the notification socket with the given user.
func notifyWSClient(t *testing.T, server *httptest.Server, user string) *websocket.Conn {
	t.Helper()

	header := http.Header{}
	if user != "" {
		header.Set("X-Notify-User", user)
	}
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"/ws/notify", header)
	if err != nil {
		t.Fatalf("ws dial (user %q) error = %v", user, err)
	}
	return conn
}

// sseStream wraps an open notification stream: the buffered reader plus
// the deadline control the underlying connection provides.
type sseStream struct {
	resp   *http.Response
	reader *bufio.Reader
	cancel context.CancelFunc
}

func (s *sseStream) close() {
	s.cancel()
	s.resp.Body.Close()
}

// readLineWithTimeout reads one line with a watchdog: client response
// bodies do not implement SetReadDeadline, so a plain ReadString can
// block forever on a stream that stopped delivering. The abandoned read
// goroutine unblocks when close() cancels the request context.
func (s *sseStream) readLineWithTimeout(d time.Duration) (string, error) {
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := s.reader.ReadString('\n')
		done <- result{line, err}
	}()

	select {
	case r := <-done:
		return r.line, r.err
	case <-time.After(d):
		return "", errors.New("sse read timed out")
	}
}

// notifySSEClient opens the notification stream with the given user and
// topics. Cleanup is registered automatically.
func notifySSEClient(t *testing.T, server *httptest.Server, user, topics string) *sseStream {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/sse/notify", nil)
	if err != nil {
		cancel()
		t.Fatalf("request error = %v", err)
	}
	if user != "" {
		req.Header.Set("X-Notify-User", user)
	}
	if topics != "" {
		req.URL.RawQuery = "topics=" + topics
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("sse connect (user %q) error = %v", user, err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		resp.Body.Close()
		t.Fatalf("sse connect (user %q) status = %d, want 200", user, resp.StatusCode)
	}

	stream := &sseStream{resp: resp, reader: bufio.NewReader(resp.Body), cancel: cancel}
	t.Cleanup(stream.close)
	return stream
}

// readSSENotification reads the next notification event from an SSE
// stream.
func readSSENotification(t *testing.T, stream *sseStream) Notification {
	t.Helper()

	eventName := ""
	for {
		line, err := stream.readLineWithTimeout(2 * time.Second)
		if err != nil {
			t.Fatalf("sse read error = %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			eventName = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if eventName != "notification" {
				continue
			}
			var note Notification
			data := strings.TrimPrefix(line, "data: ")
			if err := json.Unmarshal([]byte(data), &note); err != nil {
				t.Fatalf("unmarshal %q: %v", data, err)
			}
			return note
		}
	}
}

// readWSNotification reads the next notification event from the socket,
// failing on anything else arriving first.
func readWSNotification(t *testing.T, conn *websocket.Conn) Notification {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline error = %v", err)
	}
	var envelope struct {
		Event string       `json:"event"`
		Data  Notification `json:"data"`
	}
	if err := conn.ReadJSON(&envelope); err != nil {
		t.Fatalf("ws read error = %v", err)
	}
	if envelope.Event != "notification" {
		t.Fatalf("event = %q, want notification", envelope.Event)
	}
	return envelope.Data
}

func TestNewNotification(t *testing.T) {
	note := NewNotification("info", "Hello", "World")
	if note.ID == "" {
		t.Fatal("ID is empty")
	}
	if note.Level != "info" || note.Title != "Hello" || note.Body != "World" {
		t.Fatalf("note = %+v", note)
	}
	if note.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero")
	}
	if note.Topic != "" {
		t.Fatalf("Topic = %q, want empty on fresh notifications", note.Topic)
	}
}

func TestNotifier_NotifyAll_BothTransports(t *testing.T) {
	notifier, server := newTestNotifier(t)

	wsConn := notifyWSClient(t, server, "42")
	defer wsConn.Close()
	sse := notifySSEClient(t, server, "42", "")

	waitForUsers(t, notifier, "42")

	note := NewNotification("success", "Deployed", "v1.2.3 is live")
	notifier.NotifyAll(note)

	wsNote := readWSNotification(t, wsConn)
	sseNote := readSSENotification(t, sse)

	// Byte-identical envelope over both transports — the same ID lets a
	// page holding both deduplicate.
	if wsNote.ID != note.ID || sseNote.ID != note.ID {
		t.Fatalf("IDs differ: ws=%s sse=%s want=%s", wsNote.ID, sseNote.ID, note.ID)
	}
	if wsNote.Title != "Deployed" || sseNote.Title != "Deployed" {
		t.Fatalf("titles differ: ws=%q sse=%q", wsNote.Title, sseNote.Title)
	}
}

func TestNotifier_NotifyUser(t *testing.T) {
	notifier, server := newTestNotifier(t)

	aliceWS := notifyWSClient(t, server, "alice")
	defer aliceWS.Close()
	bobWS := notifyWSClient(t, server, "bob")
	defer bobWS.Close()
	aliceSSE := notifySSEClient(t, server, "alice", "")

	waitForUsers(t, notifier, "alice", "bob")

	note := NewNotification("warning", "Password change", "from a new device")
	if got := notifier.NotifyUser("alice", note); got != 2 {
		t.Fatalf("NotifyUser(alice) reached %d connections, want 2 (ws + sse)", got)
	}

	aliceWSNote := readWSNotification(t, aliceWS)
	aliceSSENote := readSSENotification(t, aliceSSE)
	if aliceWSNote.ID != note.ID || aliceSSENote.ID != note.ID {
		t.Fatal("alice's transports got different notifications")
	}

	// Offline and anonymous users get nothing.
	if got := notifier.NotifyUser("nobody", NewNotification("info", "x", "y")); got != 0 {
		t.Fatalf("NotifyUser(offline) reached %d, want 0", got)
	}
	if got := notifier.NotifyUser("", NewNotification("info", "x", "y")); got != 0 {
		t.Fatalf("NotifyUser(anonymous) reached %d, want 0", got)
	}

	// Bob received nothing meant for alice.
	if err := bobWS.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline error = %v", err)
	}
	if _, _, err := bobWS.ReadMessage(); err == nil {
		t.Fatal("bob received a notification meant for alice")
	}
}

func waitForUsers(t *testing.T, notifier *Notifier, want ...string) {
	t.Helper()

	sorted := func(in []string) string { return strings.Join(in, ",") }
	target := sorted(append([]string(nil), want...))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sorted(notifier.ConnectedUsers()) == target {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("ConnectedUsers() = %v, want %v", notifier.ConnectedUsers(), want)
}

func TestNotifier_NotifyTopic_WS(t *testing.T) {
	notifier, server := newTestNotifier(t)

	subscriber := notifyWSClient(t, server, "42")
	defer subscriber.Close()
	lurker := notifyWSClient(t, server, "43")
	defer lurker.Close()
	waitForUsers(t, notifier, "42", "43")

	if err := subscriber.WriteJSON(map[string]any{"event": "notify.subscribe", "data": "orders"}); err != nil {
		t.Fatalf("subscribe write error = %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && notifier.Subscribers("orders") != 1 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := notifier.Subscribers("orders"); got != 1 {
		t.Fatalf("Subscribers(orders) = %d, want 1", got)
	}

	note := NewNotification("info", "Order shipped", "#1234 on its way")
	if got := notifier.NotifyTopic("orders", note); got != 1 {
		t.Fatalf("NotifyTopic reached %d, want 1", got)
	}

	received := readWSNotification(t, subscriber)
	if received.ID != note.ID {
		t.Fatalf("subscriber got ID %s, want %s", received.ID, note.ID)
	}
	if received.Topic != "orders" {
		t.Fatalf("notification Topic = %q, want orders", received.Topic)
	}

	// The lurker never asked for the topic.
	if err := lurker.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline error = %v", err)
	}
	if _, _, err := lurker.ReadMessage(); err == nil {
		t.Fatal("lurker received a topic notification it never subscribed to")
	}

	// Unsubscribe ends deliveries.
	if err := subscriber.WriteJSON(map[string]any{"event": "notify.unsubscribe", "data": "orders"}); err != nil {
		t.Fatalf("unsubscribe write error = %v", err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && notifier.Subscribers("orders") != 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := notifier.NotifyTopic("orders", NewNotification("info", "x", "y")); got != 0 {
		t.Fatalf("NotifyTopic after unsubscribe reached %d, want 0", got)
	}
}

func TestNotifier_NotifyTopic_SSE(t *testing.T) {
	notifier, server := newTestNotifier(t)

	subscriber := notifySSEClient(t, server, "42", "orders,news")
	everyone := notifySSEClient(t, server, "43", "")
	waitForUsers(t, notifier, "42", "43")

	// Only the subscriber gets the topic event...
	note := NewNotification("info", "Order shipped", "#1234")
	if got := notifier.NotifyTopic("orders", note); got != 1 {
		t.Fatalf("NotifyTopic reached %d, want 1", got)
	}
	received := readSSENotification(t, subscriber)
	if received.Topic != "orders" {
		t.Fatalf("notification Topic = %q, want orders", received.Topic)
	}

	// ...while NotifyAll still reaches both streams.
	notifier.NotifyAll(NewNotification("success", "Deployed", "v1.2.3"))
	readSSENotification(t, subscriber)
	readSSENotification(t, everyone)

	// A non-notification event on the broker passes through untouched.
	notifier.sse.Broadcast(SSEEvent{Event: "app.ping", Data: []byte(`{"t":"now"}`)})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, err := everyone.readLineWithTimeout(2 * time.Second)
		if err != nil {
			t.Fatalf("sse read error = %v", err)
		}
		if strings.Contains(line, "event: app.ping") {
			break
		}
	}
}

func TestNotifier_SSEInvalidTopic(t *testing.T) {
	_, server := newTestNotifier(t)

	resp, err := http.Get(server.URL + "/sse/notify?topics=has%20spaces")
	if err != nil {
		t.Fatalf("GET error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 on invalid topic", resp.StatusCode)
	}
}

func TestNotifier_InvalidTopicSubscribe(t *testing.T) {
	notifier, server := newTestNotifier(t)

	conn := notifyWSClient(t, server, "42")
	defer conn.Close()
	waitForUsers(t, notifier, "42")

	// Names outside the pattern are silently ignored (routing keys, not
	// free text): the subscription never registers.
	for _, bad := range []string{"has spaces", "slash/name", strings.Repeat("x", 65)} {
		if err := conn.WriteJSON(map[string]any{"event": "notify.subscribe", "data": bad}); err != nil {
			t.Fatalf("subscribe write error = %v", err)
		}
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if notifier.Subscribers("has spaces")+notifier.Subscribers("slash/name")+notifier.Subscribers(strings.Repeat("x", 65)) > 0 {
			t.Fatal("invalid topic name was registered")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := notifier.NotifyTopic(strings.Repeat("x", 65), NewNotification("info", "x", "y")); got != 0 {
		t.Fatalf("NotifyTopic(invalid) reached %d, want 0", got)
	}
}

func TestNotifier_AnonymousClients(t *testing.T) {
	notifier, server := newTestNotifier(t)

	// No X-Notify-User header: anonymous on both transports.
	wsConn := notifyWSClient(t, server, "")
	defer wsConn.Close()
	sse := notifySSEClient(t, server, "", "")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(notifier.ConnectedUsers()) != 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := notifier.ConnectedUsers(); len(got) != 0 {
		t.Fatalf("anonymous clients registered as users: %v", got)
	}

	// Anonymous clients are not user-targetable, but NotifyAll still
	// reaches them.
	if got := notifier.NotifyUser("", NewNotification("info", "x", "y")); got != 0 {
		t.Fatalf("NotifyUser(anonymous) reached %d, want 0", got)
	}

	note := NewNotification("info", "Everyone", "hello")
	notifier.NotifyAll(note)
	if got := readWSNotification(t, wsConn); got.ID != note.ID {
		t.Fatalf("anonymous ws client got ID %s, want %s", got.ID, note.ID)
	}
	if got := readSSENotification(t, sse); got.ID != note.ID {
		t.Fatalf("anonymous sse client got ID %s, want %s", got.ID, note.ID)
	}
}

func TestNotifier_DisconnectCleanup(t *testing.T) {
	notifier, server := newTestNotifier(t)

	wsConn := notifyWSClient(t, server, "42")
	sse := notifySSEClient(t, server, "42", "orders")
	waitForUsers(t, notifier, "42")
	if got := notifier.Subscribers("orders"); got != 1 {
		t.Fatalf("Subscribers(orders) = %d, want 1", got)
	}

	// Drop both connections (the hostile path: raw TCP close, no
	// protocol-level goodbye).
	wsConn.UnderlyingConn().Close()
	sse.close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) &&
		(len(notifier.ConnectedUsers()) != 0 || notifier.Subscribers("orders") != 0) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := notifier.ConnectedUsers(); len(got) != 0 {
		t.Fatalf("registries kept stale users: %v", got)
	}
	if got := notifier.Subscribers("orders"); got != 0 {
		t.Fatalf("registries kept stale subscribers: %d", got)
	}

	// A send to the vanished user reaches nobody.
	if got := notifier.NotifyUser("42", NewNotification("info", "x", "y")); got != 0 {
		t.Fatalf("NotifyUser after disconnect reached %d, want 0", got)
	}
}

func TestNotifier_IdentityIsServerSide(t *testing.T) {
	notifier, server := newTestNotifier(t)

	conn := notifyWSClient(t, server, "42")
	defer conn.Close()
	waitForUsers(t, notifier, "42")

	// A client cannot change its identity by message: control events
	// carry topics only, and the identity came from the handshake.
	if err := conn.WriteJSON(map[string]any{"event": "notify.subscribe", "data": "orders"}); err != nil {
		t.Fatalf("subscribe write error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && notifier.Subscribers("orders") != 1 {
		time.Sleep(5 * time.Millisecond)
	}

	users := notifier.ConnectedUsers()
	if len(users) != 1 || users[0] != "42" {
		t.Fatalf("identity changed by client messages: %v", users)
	}
}

func TestNotifier_ConcurrentChurn(t *testing.T) {
	notifier, server := newTestNotifier(t)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Senders keep notifying while clients churn.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					notifier.NotifyAll(NewNotification("info", "tick", "tock"))
					notifier.NotifyTopic("orders", NewNotification("info", "tick", "tock"))
					notifier.NotifyUser("churn", NewNotification("info", "tick", "tock"))
					time.Sleep(2 * time.Millisecond)
				}
			}
		}()
	}

	// Churning clients on both transports, some subscribing mid-flight.
	var churn sync.WaitGroup
	for i := 0; i < 12; i++ {
		churn.Add(1)
		go func(i int) {
			defer churn.Done()
			user := "churn"
			if i%2 == 0 {
				user = ""
			}
			conn := notifyWSClient(t, server, user)
			_ = conn.WriteJSON(map[string]any{"event": "notify.subscribe", "data": "orders"})
			conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			for {
				var ev map[string]any
				if err := conn.ReadJSON(&ev); err != nil {
					break
				}
			}
			conn.UnderlyingConn().Close()
		}(i)
		for j := 0; j < 4; j++ {
			churn.Add(1)
			go func() {
				defer churn.Done()
				stream := notifySSEClient(t, server, "churn", "orders")
				deadline := time.Now().Add(50 * time.Millisecond)
				for time.Now().Before(deadline) {
					if _, err := stream.readLineWithTimeout(100 * time.Millisecond); err != nil {
						break
					}
				}
				stream.close()
			}()
		}
	}
	churn.Wait()
	close(stop)
	wg.Wait()

	// Every connection is gone: the registries must drain.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) &&
		(len(notifier.ConnectedUsers()) != 0 || notifier.Subscribers("orders") != 0) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := notifier.ConnectedUsers(); len(got) != 0 {
		t.Fatalf("registries kept stale users after churn: %v", got)
	}
	if got := notifier.Subscribers("orders"); got != 0 {
		t.Fatalf("registries kept stale subscribers after churn: %d", got)
	}
}

func TestParseNotifyTopics(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{"empty", "", nil, false},
		{"spaces only", "   ", nil, false},
		{"single", "orders", []string{"orders"}, false},
		{"csv with spaces", " orders , news ", []string{"orders", "news"}, false},
		{"dotted and dashed", "order.1, team-updates", []string{"order.1", "team-updates"}, false},
		{"invalid name", "bad name", nil, true},
		{"too long", strings.Repeat("x", 65), nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topics, err := parseNotifyTopics(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseNotifyTopics(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidTopic) {
					t.Fatalf("error = %v, want ErrInvalidTopic", err)
				}
				return
			}
			if len(topics) != len(tt.want) {
				t.Fatalf("topics = %v, want %v", topics, tt.want)
			}
			for _, w := range tt.want {
				if _, ok := topics[w]; !ok {
					t.Fatalf("topics %v missing %q", topics, w)
				}
			}
		})
	}
}
