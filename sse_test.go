package regius

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSSEBroker_SubscribeAndBroadcast(t *testing.T) {
	broker := NewSSEBroker()
	ch1, unsub1 := broker.Subscribe(context.Background())
	defer unsub1()
	ch2, unsub2 := broker.Subscribe(context.Background())
	defer unsub2()

	broker.Broadcast(SSEEvent{
		Event: "update",
		Data:  []byte(`{"hello":"world"}`),
	})

	for i, ch := range []<-chan SSEEvent{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Event != "update" {
				t.Errorf("client %d: event = %q, want %q", i, ev.Event, "update")
			}
			if string(ev.Data) != `{"hello":"world"}` {
				t.Errorf("client %d: data = %q, want %q", i, string(ev.Data), `{"hello":"world"}`)
			}
		case <-time.After(time.Second):
			t.Fatalf("client %d: timed out waiting for event", i)
		}
	}
}

func TestSSEBroker_Send(t *testing.T) {
	broker := NewSSEBroker()
	ch, unsub := broker.Subscribe(context.Background())
	defer unsub()

	clientID := ""
	broker.mu.RLock()
	for id := range broker.clients {
		clientID = id
		break
	}
	broker.mu.RUnlock()

	if clientID == "" {
		t.Fatal("no client id found")
	}

	if err := broker.Send(clientID, SSEEvent{Event: "direct", Data: []byte("ping")}); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.Event != "direct" || string(ev.Data) != "ping" {
			t.Errorf("event = %q/%q, want direct/ping", ev.Event, string(ev.Data))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for direct event")
	}
}

func TestSSEBroker_SendUnknownClient(t *testing.T) {
	broker := NewSSEBroker()
	err := broker.Send("unknown", SSEEvent{Event: "x", Data: []byte("y")})
	if err == nil {
		t.Fatal("expected error for unknown client")
	}
}

func TestSSEBroker_Unsubscribe(t *testing.T) {
	broker := NewSSEBroker()
	ch, unsub := broker.Subscribe(context.Background())
	unsub()

	broker.Broadcast(SSEEvent{Event: "lost", Data: []byte("data")})

	select {
	case <-ch:
		t.Fatal("received event after unsubscribe")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSSEBroker_Handler(t *testing.T) {
	broker := NewSSEBroker()
	server := httptest.NewServer(broker.Handler())
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		broker.Broadcast(SSEEvent{
			ID:    "1",
			Event: "heartbeat",
			Data:  []byte(`{"time":"now"}`),
		})
	}()

	reader := bufio.NewReader(resp.Body)
	var lines []string
	for len(lines) < 4 {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed to read line: %v", err)
		}
		lines = append(lines, line)
	}

	cancel()
	resp.Body.Close()

	body := strings.Join(lines, "")
	want := []string{
		"id: 1",
		"event: heartbeat",
		"data: {\"time\":\"now\"}",
		"",
	}
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("body missing %q\nbody:\n%s", w, body)
		}
	}
}

func TestRegius_SSEBroadcastJSON(t *testing.T) {
	r := &Regius{SSE: NewSSEBroker()}
	ch, unsub := r.SSE.Subscribe(context.Background())
	defer unsub()

	payload := map[string]string{"message": "hello"}
	if err := r.SSEBroadcastJSON("ping", payload); err != nil {
		t.Fatalf("SSEBroadcastJSON failed: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.Event != "ping" {
			t.Errorf("event = %q, want ping", ev.Event)
		}
		if string(ev.Data) != `{"message":"hello"}` {
			t.Errorf("data = %q, want %q", string(ev.Data), `{"message":"hello"}`)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestRegius_SSEBroadcastJSON_NotInitialized(t *testing.T) {
	r := &Regius{}
	if err := r.SSEBroadcastJSON("ping", map[string]string{}); err == nil {
		t.Fatal("expected error when SSE broker is nil")
	}
}
