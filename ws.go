package regius

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hbarral/regius/ws"
)

// wsConfig holds the WebSocket configuration resolved from the WS_*
// environment variables. The hub is always constructed (so apps can mount
// r.WS.Handler anywhere), but the default route at Path is only mounted
// while enabled.
type wsConfig struct {
	enabled          bool
	path             string
	allowedOrigins   []string
	allowEmptyOrigin bool
	heartbeat        time.Duration
	writeTimeout     time.Duration
	pongTimeout      time.Duration
	maxMessageSize   int64
	clientBuffer     int
	maxClients       int64
}

func (r *Regius) createWSConfig() wsConfig {
	enabled := false
	if v := os.Getenv("WS_ENABLED"); v != "" {
		enabled, _ = strconv.ParseBool(v)
	}

	path := strings.TrimSpace(os.Getenv("WS_PATH"))
	if path == "" {
		path = "/ws"
	}

	var allowedOrigins []string
	if v := strings.TrimSpace(os.Getenv("WS_ALLOWED_ORIGINS")); v != "" {
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				allowedOrigins = append(allowedOrigins, o)
			}
		}
	}

	allowEmptyOrigin := true
	if v := os.Getenv("WS_ALLOW_EMPTY_ORIGIN"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			allowEmptyOrigin = parsed
		}
	}

	maxMessageSize, _ := strconv.ParseInt(os.Getenv("WS_MAX_MESSAGE_SIZE"), 10, 64)
	if maxMessageSize <= 0 {
		maxMessageSize = 32 * 1024
	}

	clientBuffer, _ := strconv.Atoi(os.Getenv("WS_CLIENT_BUFFER"))
	if clientBuffer <= 0 {
		clientBuffer = 16
	}

	maxClients, _ := strconv.ParseInt(os.Getenv("WS_MAX_CLIENTS"), 10, 64)
	if maxClients < 0 {
		maxClients = 0
	}

	return wsConfig{
		enabled:          enabled,
		path:             path,
		allowedOrigins:   allowedOrigins,
		allowEmptyOrigin: allowEmptyOrigin,
		heartbeat:        parseDurationEnv("WS_HEARTBEAT", 30*time.Second),
		writeTimeout:     parseDurationEnv("WS_WRITE_TIMEOUT", 10*time.Second),
		pongTimeout:      parseDurationEnv("WS_PONG_TIMEOUT", 60*time.Second),
		maxMessageSize:   maxMessageSize,
		clientBuffer:     clientBuffer,
		maxClients:       maxClients,
	}
}

// createWSHub builds the always-on WebSocket hub with the WS_* options.
func (r *Regius) createWSHub() *ws.Hub {
	return ws.NewHub(
		ws.WithClientBuffer(r.config.ws.clientBuffer),
		ws.WithHeartbeat(r.config.ws.heartbeat),
		ws.WithWriteTimeout(r.config.ws.writeTimeout),
		ws.WithPongTimeout(r.config.ws.pongTimeout),
		ws.WithMaxMessageSize(r.config.ws.maxMessageSize),
		ws.WithMaxClients(r.config.ws.maxClients),
	)
}

// createWSUpgrader builds the upgrader used by the default route mount:
// same-origin checks (the default policy), extended by WS_ALLOWED_ORIGINS,
// with WS_ALLOW_EMPTY_ORIGIN=false requiring an Origin header from every
// client. Apps mounting their own endpoints build their own Upgrader.
func (r *Regius) createWSUpgrader() *ws.Upgrader {
	checkOrigin := ws.CheckSameOrigin
	if len(r.config.ws.allowedOrigins) > 0 {
		checkOrigin = ws.AllowOrigins(r.config.ws.allowedOrigins...)
	}
	if !r.config.ws.allowEmptyOrigin {
		checkOrigin = ws.RequireOrigin(checkOrigin)
	}

	return &ws.Upgrader{CheckOrigin: checkOrigin}
}

// WSBroadcastJSON marshals payload and broadcasts it to every connected
// WebSocket client on the framework hub, mirroring SSEBroadcastJSON: the
// wire envelope is the same {"event", "data"} shape, so a payload works
// over both transports unchanged.
func (r *Regius) WSBroadcastJSON(event string, payload interface{}) error {
	if r.WS == nil {
		return errors.New("websocket hub not initialized")
	}

	ev, err := ws.NewEvent(event, payload)
	if err != nil {
		return fmt.Errorf("failed to marshal websocket payload: %w", err)
	}

	r.WS.Broadcast(ev)
	return nil
}
