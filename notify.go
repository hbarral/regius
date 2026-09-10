package regius

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/hbarral/regius/ws"
)

// Notification is a typed, UI-ready notification. Its JSON shape is the
// browser contract and is byte-identical over both transports: one
// listener can serve SSE and WebSocket clients with the same toast
// function. The ID exists for client-side deduplication — a page holding
// both an EventSource and a WebSocket receives NotifyAll twice.
type Notification struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Level     string    `json:"level"` // info | success | warning | error — free-form UI hint
	Link      string    `json:"link,omitempty"`
	Topic     string    `json:"topic,omitempty"` // set on topic deliveries; user/all sends leave it empty
	CreatedAt time.Time `json:"created_at"`
}

// NewNotification builds a Notification with a generated ID and timestamp.
func NewNotification(level, title, body string) Notification {
	return Notification{
		ID:        xid.New().String(),
		Level:     level,
		Title:     title,
		Body:      body,
		CreatedAt: time.Now(),
	}
}

// notifyEventName is the wire event name on both transports.
const notifyEventName = "notification"

// notifyUserMetaKey carries the connection's user identity in ws
// ClientInfo.Meta (set during the handshake by the notifier's mount).
const notifyUserMetaKey = "notify.user"

// Client → server control events on the notification socket.
const (
	notifySubscribeEvent   = "notify.subscribe"
	notifyUnsubscribeEvent = "notify.unsubscribe"
)

// notifyTopicPattern constrains topic names: routing keys, not free text.
var notifyTopicPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)

// ErrInvalidTopic is returned when a topic name fails notifyTopicPattern.
var ErrInvalidTopic = errors.New("regius: invalid notification topic (want 1-64 of a-z A-Z 0-9 _ . -)")

// notifierEntry is the notifier's bookkeeping for one connected client
// (per transport): the identity its handshake carried and the topics it
// subscribed to.
type notifierEntry struct {
	user   string
	topics map[string]struct{}
}

// Notifier delivers typed notifications over the app's SSE broker and
// WebSocket hub. Construct it with NewNotifier (Regius wires one as
// r.Notifier); mount its handlers on the app routes, where the session
// rides the request:
//
//	a.get("/sse/notify", app.Notifier.SSEHandler())
//	a.get("/ws/notify", app.Notifier.WSHandler(nil))
//
// Delivery is best-effort and in-memory: a user with zero connections
// gets nothing (NotifyUser returns the number of connections reached),
// and there is no offline inbox. Registries are single-process, like the
// hub itself.
//
// The notifier never replaces the hub's own listeners: it registers its
// connect/disconnect/message listeners additively, so apps using the hub
// hooks directly keep working.
type Notifier struct {
	sse      *SSEBroker
	ws       *ws.Hub
	identity func(*http.Request) string

	mu         sync.Mutex
	wsClients  map[string]*notifierEntry // ws client ID → entry
	sseClients map[string]*notifierEntry // sse client ID → entry
}

// NewNotifier builds a Notifier over the given broker and hub. identity
// extracts the connection's user key from the handshake request; nil
// means every connection is anonymous (user targeting then always
// reaches nobody — only NotifyAll and topics work). The default Regius
// wiring reads the auth scaffolding's "userID" session key.
func NewNotifier(sse *SSEBroker, hub *ws.Hub, identity func(*http.Request) string) *Notifier {
	n := &Notifier{
		sse:        sse,
		ws:         hub,
		identity:   identity,
		wsClients:  make(map[string]*notifierEntry),
		sseClients: make(map[string]*notifierEntry),
	}

	// Additive listeners: the notifier coexists with app-installed hooks.
	hub.OnConnect(n.onWSConnect)
	hub.OnDisconnect(n.onWSDisconnect)
	hub.OnMessage(n.onWSMessage)

	return n
}

// identityOf resolves the connection's user key.
func (n *Notifier) identityOf(r *http.Request) string {
	if n.identity == nil {
		return ""
	}
	return n.identity(r)
}

// WSHandler returns the http.HandlerFunc for the notification socket.
// Clients subscribe and unsubscribe by sending
// {"event":"notify.subscribe","data":"<topic>"} messages; the identity
// comes from the handshake (server-side, never from client messages). A
// nil upgrader uses the package default (same-origin checks).
func (n *Notifier) WSHandler(u *ws.Upgrader) http.HandlerFunc {
	return n.ws.HandlerWithMeta(u, func(r *http.Request) map[string]string {
		return map[string]string{notifyUserMetaKey: n.identityOf(r)}
	})
}

// SSEHandler returns the http.HandlerFunc for the notification stream.
// Because SSE is one-way, topics come from the query string
// (?topics=orders,news) and are fixed for the connection's life. Non-
// notification events on the broker pass through untouched, so a client
// may also consume the app's own SSE broadcasts on the same stream.
func (n *Notifier) SSEHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		topics, err := parseNotifyTopics(r.URL.Query().Get("topics"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		clientID, events, unsubscribe := n.sse.SubscribeWithID(r.Context())
		defer unsubscribe()

		entry := &notifierEntry{user: n.identityOf(r), topics: topics}
		n.mu.Lock()
		n.sseClients[clientID] = entry
		n.mu.Unlock()
		defer func() {
			n.mu.Lock()
			delete(n.sseClients, clientID)
			n.mu.Unlock()
		}()

		for {
			select {
			case <-r.Context().Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				if !sseWantsEvent(entry, ev) {
					continue
				}
				if err := writeEvent(w, ev); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

// NotifyAll delivers note to every connection on both transports: one
// SSE broadcast plus one WebSocket broadcast. A page holding both
// receives it twice — deduplicate client-side by Notification.ID.
func (n *Notifier) NotifyAll(note Notification) {
	sseEv, wsEv := notifyEnvelope(note)
	n.sse.Broadcast(sseEv)
	n.ws.Broadcast(wsEv)
}

// NotifyUser delivers note to every connection whose handshake carried
// the given user key. It returns the number of connections reached —
// zero means the user is offline, and (best-effort, in-memory) the
// notification is gone; see the Notifier docs.
func (n *Notifier) NotifyUser(userID string, note Notification) int {
	if userID == "" {
		return 0
	}
	sseEv, wsEv := notifyEnvelope(note)
	return n.sendWS(func(entry *notifierEntry) bool { return entry.user == userID }, wsEv) +
		n.sendSSE(func(entry *notifierEntry) bool { return entry.user == userID }, sseEv)
}

// NotifyTopic delivers note to every connection subscribed to topic
// (carrying the topic name in the notification's Topic field). Topics
// are opt-in public channels — any connected client may subscribe to any
// topic — so they are routing, not access control. Returns the number of
// connections reached.
func (n *Notifier) NotifyTopic(topic string, note Notification) int {
	if !notifyTopicPattern.MatchString(topic) {
		return 0
	}
	note.Topic = topic
	sseEv, wsEv := notifyEnvelope(note)
	return n.sendWS(func(entry *notifierEntry) bool {
		_, ok := entry.topics[topic]
		return ok
	}, wsEv) + n.sendSSE(func(entry *notifierEntry) bool {
		_, ok := entry.topics[topic]
		return ok
	}, sseEv)
}

// Subscribers reports how many connections currently listen to topic
// (both transports combined).
func (n *Notifier) Subscribers(topic string) int {
	n.mu.Lock()
	defer n.mu.Unlock()

	count := 0
	for _, entry := range n.wsClients {
		if _, ok := entry.topics[topic]; ok {
			count++
		}
	}
	for _, entry := range n.sseClients {
		if _, ok := entry.topics[topic]; ok {
			count++
		}
	}
	return count
}

// ConnectedUsers lists the distinct user keys with at least one
// connection (both transports), sorted for stable output.
func (n *Notifier) ConnectedUsers() []string {
	n.mu.Lock()
	defer n.mu.Unlock()

	seen := make(map[string]struct{})
	for _, entry := range n.wsClients {
		if entry.user != "" {
			seen[entry.user] = struct{}{}
		}
	}
	for _, entry := range n.sseClients {
		if entry.user != "" {
			seen[entry.user] = struct{}{}
		}
	}
	users := make([]string, 0, len(seen))
	for u := range seen {
		users = append(users, u)
	}
	sort.Strings(users)
	return users
}

// onWSConnect records a hub client's identity from its handshake Meta.
// Clients from other mounts (no Meta key) are simply not user-targetable.
func (n *Notifier) onWSConnect(info ws.ClientInfo) {
	user := info.Meta[notifyUserMetaKey]
	if user == "" {
		return
	}
	n.mu.Lock()
	n.wsClients[info.ID] = &notifierEntry{user: user, topics: make(map[string]struct{})}
	n.mu.Unlock()
}

// onWSDisconnect drops the client's bookkeeping when it leaves the hub.
func (n *Notifier) onWSDisconnect(info ws.ClientInfo) {
	n.mu.Lock()
	delete(n.wsClients, info.ID)
	n.mu.Unlock()
}

// onWSMessage handles the subscribe/unsubscribe control events. Any other
// event is ignored — the notification socket is not a general-purpose
// channel. The user identity is never taken from client messages.
func (n *Notifier) onWSMessage(info ws.ClientInfo, ev ws.Event) {
	switch ev.Event {
	case notifySubscribeEvent, notifyUnsubscribeEvent:
	default:
		return
	}

	topic := decodeNotifyTopic(ev.Data)
	if topic == "" || !notifyTopicPattern.MatchString(topic) {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	entry, ok := n.wsClients[info.ID]
	if !ok {
		// Not a notifier client (e.g. the default /ws mount): control
		// events are meaningless for it.
		return
	}
	if ev.Event == notifySubscribeEvent {
		entry.topics[topic] = struct{}{}
	} else {
		delete(entry.topics, topic)
	}
}

// sendWS delivers ev to the hub clients whose entry matches, pruning
// stale entries (the hub already forgot them) along the way. Sends are
// non-blocking, so holding the registry mutex is safe.
func (n *Notifier) sendWS(match func(*notifierEntry) bool, ev ws.Event) int {
	n.mu.Lock()
	defer n.mu.Unlock()

	count := 0
	var stale []string
	for id, entry := range n.wsClients {
		if !match(entry) {
			continue
		}
		if err := n.ws.Send(id, ev); err != nil {
			if errors.Is(err, ws.ErrClientNotFound) {
				stale = append(stale, id)
			}
			continue
		}
		count++
	}
	for _, id := range stale {
		delete(n.wsClients, id)
	}
	return count
}

// sendSSE delivers ev to the broker subscribers whose entry matches; see
// sendWS for the pruning and locking rationale.
func (n *Notifier) sendSSE(match func(*notifierEntry) bool, ev SSEEvent) int {
	n.mu.Lock()
	defer n.mu.Unlock()

	count := 0
	var stale []string
	for id, entry := range n.sseClients {
		if !match(entry) {
			continue
		}
		if err := n.sse.Send(id, ev); err != nil {
			// "client not found" — the stream ended and its deferred
			// cleanup hasn't run yet.
			stale = append(stale, id)
			continue
		}
		count++
	}
	for _, id := range stale {
		delete(n.sseClients, id)
	}
	return count
}

// notifyEnvelope encodes note into the shared {"event","data"} wire
// shape for both transports.
func notifyEnvelope(note Notification) (SSEEvent, ws.Event) {
	data, err := json.Marshal(note)
	if err != nil {
		// Notification fields are all strings and time.Time: marshal
		// cannot fail. Keep a defensive fallback rather than panicking.
		data = []byte(`{"id":"","title":` + mustMarshalString(note.Title) + `}`)
	}
	return SSEEvent{Event: notifyEventName, Data: data},
		ws.Event{Event: notifyEventName, Data: data}
}

func mustMarshalString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// parseNotifyTopics parses the SSE ?topics= query parameter. An empty
// parameter is fine (no topics); an invalid name is a 400.
func parseNotifyTopics(raw string) (map[string]struct{}, error) {
	topics := make(map[string]struct{})
	if strings.TrimSpace(raw) == "" {
		return topics, nil
	}
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !notifyTopicPattern.MatchString(name) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidTopic, name)
		}
		topics[name] = struct{}{}
	}
	return topics, nil
}

// decodeNotifyTopic extracts the topic name from a control message's
// data: a JSON-encoded string ("orders"), or (leniently) raw bytes.
func decodeNotifyTopic(data []byte) string {
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return ""
	}
	var topic string
	if err := json.Unmarshal([]byte(raw), &topic); err == nil {
		return topic
	}
	return strings.Trim(raw, `"`)
}

// sseWantsEvent decides whether an SSE stream with the given entry
// should receive ev: topic-scoped notifications only reach subscribers,
// everything else on the broker passes through.
func sseWantsEvent(entry *notifierEntry, ev SSEEvent) bool {
	if ev.Event != notifyEventName {
		return true
	}
	var probe struct {
		Topic string `json:"topic"`
	}
	if err := json.Unmarshal(ev.Data, &probe); err != nil || probe.Topic == "" {
		return true
	}
	_, ok := entry.topics[probe.Topic]
	return ok
}
