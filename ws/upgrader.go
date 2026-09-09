// Package ws provides WebSocket support for Regius applications.
//
// It wraps github.com/gorilla/websocket (the de-facto standard Go
// implementation) so that gorilla types never leak into application code:
// [Upgrader] performs the handshake, [Conn] wraps an upgraded connection
// with Regius-shaped read/write semantics, and [Hub] mirrors the framework's
// SSE broker (register/broadcast/send with bounded per-client buffers).
//
// The wire envelope is [Event], which carries the same {"event", "data"}
// shape the SSE broker puts on the wire, so payloads work over both
// transports unchanged.
//
// Upgrade rejections are typed: [ErrNotWebSocket], [ErrOriginRejected],
// [ErrUpgradeFailed].
package ws

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Handshake errors returned by [Upgrader.Upgrade].
var (
	// ErrNotWebSocket is returned when the request is not a WebSocket
	// handshake (wrong method, or missing Upgrade/Connection headers).
	ErrNotWebSocket = errors.New("regius/ws: request is not a websocket handshake")

	// ErrOriginRejected is returned when the request's Origin is not
	// allowed by the configured origin check (CSWSH protection).
	ErrOriginRejected = errors.New("regius/ws: request origin rejected")

	// ErrUpgradeFailed wraps the underlying handshake failure returned by
	// the gorilla upgrader (unsupported version, bad challenge key, ...).
	// The original error is preserved for errors.Is/As inspection.
	ErrUpgradeFailed = errors.New("regius/ws: websocket upgrade failed")
)

// defaultWriteTimeout bounds every server-side write when an Upgrader leaves
// WriteTimeout at its zero value.
const defaultWriteTimeout = 10 * time.Second

// Upgrader upgrades HTTP requests to WebSocket connections with
// Regius-shaped defaults. The zero value is ready to use: same-origin
// checks (empty Origin allowed), no subprotocol negotiation, no
// compression, and a 10s write timeout.
type Upgrader struct {
	// CheckOrigin decides whether the handshake's Origin header is
	// allowed. When nil, [CheckSameOrigin] is used. A nil func together
	// with an absent Origin header permits the handshake: non-browser
	// clients (CLI tools, server-to-server) do not send Origin. See
	// [AllowOrigins] to build an explicit allowlist.
	CheckOrigin func(r *http.Request) error

	// Subprotocols lists the subprotocols the server is willing to
	// negotiate; the first client-requested match wins.
	Subprotocols []string

	// Compression enables permessage-deflate on the upgraded connection.
	Compression bool

	// WriteTimeout bounds writes on connections produced by this
	// upgrader. Zero means the 10s package default.
	WriteTimeout time.Duration
}

// Upgrade performs the WebSocket handshake and returns the wrapped
// connection. Failed handshakes write the corresponding HTTP error response
// (403 for rejected origins, 400/405 for malformed handshakes) before
// returning a typed error.
func (u *Upgrader) Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if r.Method != http.MethodGet || !websocket.IsWebSocketUpgrade(r) {
		http.Error(w, ErrNotWebSocket.Error(), http.StatusBadRequest)
		return nil, ErrNotWebSocket
	}

	checkOrigin := u.CheckOrigin
	if checkOrigin == nil {
		checkOrigin = CheckSameOrigin
	}
	if err := checkOrigin(r); err != nil {
		http.Error(w, ErrOriginRejected.Error(), http.StatusForbidden)
		return nil, fmt.Errorf("%w: %w", ErrOriginRejected, err)
	}

	writeTimeout := u.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = defaultWriteTimeout
	}

	gorilla := &websocket.Upgrader{
		// The origin check already ran above with Regius semantics.
		CheckOrigin:       func(*http.Request) bool { return true },
		Subprotocols:      u.Subprotocols,
		EnableCompression: u.Compression,
	}

	conn, err := gorilla.Upgrade(w, r, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUpgradeFailed, err)
	}

	return &Conn{ws: conn, writeTimeout: writeTimeout}, nil
}

// RequireOrigin wraps checker so that requests without an Origin header
// are rejected. Use it when only browser clients are expected: non-browser
// clients (CLI tools, server-to-server) do not send Origin, and the default
// policies admit them.
func RequireOrigin(checker func(r *http.Request) error) func(r *http.Request) error {
	return func(r *http.Request) error {
		if strings.TrimSpace(r.Header.Get("Origin")) == "" {
			return errors.New("origin header required")
		}
		return checker(r)
	}
}

// CheckSameOrigin is the default origin policy: it accepts requests with no
// Origin header (non-browser clients) and requests whose Origin host equals
// the request's Host header. Any other Origin is rejected — a malicious page
// can open a WebSocket to this server riding the victim's cookies
// (cross-site WebSocket hijacking), so cross-origin handshakes must be
// opted into explicitly via [AllowOrigins].
func CheckSameOrigin(r *http.Request) error {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return nil
	}

	u, err := url.Parse(origin)
	if err != nil {
		return errors.New("invalid origin header")
	}
	if strings.EqualFold(u.Host, r.Host) {
		return nil
	}

	return errors.New("origin host does not match request host")
}

// AllowOrigins returns a CheckOrigin that accepts the given origins.
// Entries may be bare hosts ("example.com"), scheme://host URLs
// ("https://example.com"), or "*" to accept any origin. A "*" entry also
// disables the same-origin fallback: every handshake must match a listed
// origin or carry no Origin header at all.
func AllowOrigins(origins ...string) func(r *http.Request) error {
	normalized := make([]string, 0, len(origins))
	allowAll := false
	for _, o := range origins {
		o = strings.TrimSpace(strings.ToLower(o))
		if o == "" {
			continue
		}
		if o == "*" {
			allowAll = true
			continue
		}
		if i := strings.Index(o, "://"); i >= 0 {
			o = o[i+3:]
		}
		o = strings.TrimSuffix(o, "/")
		normalized = append(normalized, o)
	}

	return func(r *http.Request) error {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			return nil
		}

		if allowAll {
			return nil
		}

		u, err := url.Parse(origin)
		if err != nil {
			return errors.New("invalid origin header")
		}

		host := strings.ToLower(u.Host)
		for _, allowed := range normalized {
			if host == allowed {
				return nil
			}
		}

		return errors.New("origin not in allowlist")
	}
}
