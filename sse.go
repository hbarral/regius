package regius

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
)

type SSEEvent struct {
	ID    string
	Event string
	Data  []byte
	Retry int
}

type SSEBroker struct {
	mu      sync.RWMutex
	clients map[string]chan SSEEvent
	nextID  atomic.Int64
}

func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients: make(map[string]chan SSEEvent),
	}
}

func (b *SSEBroker) Subscribe(ctx context.Context) (<-chan SSEEvent, func()) {
	id := b.nextID.Add(1)
	clientID := strconv.FormatInt(id, 10)
	ch := make(chan SSEEvent, 16)

	b.mu.Lock()
	b.clients[clientID] = ch
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		delete(b.clients, clientID)
		b.mu.Unlock()
	}
}

func (b *SSEBroker) Broadcast(ev SSEEvent) {
	b.mu.RLock()
	clients := make([]chan SSEEvent, 0, len(b.clients))
	for _, ch := range b.clients {
		clients = append(clients, ch)
	}
	b.mu.RUnlock()

	for _, ch := range clients {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (b *SSEBroker) Send(clientID string, ev SSEEvent) error {
	b.mu.RLock()
	ch, ok := b.clients[clientID]
	b.mu.RUnlock()
	if !ok {
		return errors.New("client not found")
	}

	select {
	case ch <- ev:
		return nil
	default:
		return errors.New("client buffer full")
	}
}

func (b *SSEBroker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		events, unsubscribe := b.Subscribe(r.Context())
		defer unsubscribe()

		for {
			select {
			case <-r.Context().Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				if err := writeEvent(w, ev); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

func (r *Regius) SSEBroadcastJSON(event string, payload interface{}) error {
	if r.SSE == nil {
		return errors.New("SSE broker not initialized")
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal SSE payload: %w", err)
	}

	r.SSE.Broadcast(SSEEvent{
		Event: event,
		Data:  data,
	})

	return nil
}

func writeEvent(w io.Writer, ev SSEEvent) error {
	if ev.ID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", ev.ID); err != nil {
			return err
		}
	}
	if ev.Event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", ev.Event); err != nil {
			return err
		}
	}
	if ev.Retry > 0 {
		if _, err := fmt.Fprintf(w, "retry: %d\n", ev.Retry); err != nil {
			return err
		}
	}
	for _, line := range bytes.Split(ev.Data, []byte("\n")) {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "\n")
	return err
}
