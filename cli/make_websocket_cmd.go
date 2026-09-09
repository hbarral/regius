package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var makeWebsocketCmd = &cobra.Command{
	Use:   "websocket [name]",
	Short: "Create a WebSocket endpoint",
	Long: `Creates a WebSocket endpoint in the handlers directory: an upgrade
handler with a read loop and TODO markers, plus a broadcast helper.

The route is mounted at /ws/<name> on the app routes (not the framework's
default /ws mount), so the handshake runs through the session/CSRF
middleware and the session cookie is available for authenticating the
upgrade. The generated handler echoes events back so the socket is
testable immediately (websocat ws://localhost:PORT/ws/<name>).

Broadcast to every connected socket from anywhere with
handlers.WSBroadcast<Name>(app, event, payload).`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeWebsocket(args[0]); err != nil {
			exitGracefully(err)
		}
		color.Green("WebSocket endpoint created!")
	},
}

func doMakeWebsocket(name string) error {
	if name == "" {
		return errors.New("you must give the websocket a name")
	}

	lower := strings.ToLower(name)
	title := pascalIdent(lower)
	wsName := snakeIdent(name)

	// the handler file; refuse duplicates
	fileName := b.RootPath + "/handlers/ws_" + wsName + ".go"
	if fileExists(fileName) {
		return errors.New(fileName + " already exists!")
	}

	data, err := templateFS.ReadFile("templates/websockets/handler.go.tmpl")
	if err != nil {
		return err
	}
	src := string(data)
	src = strings.ReplaceAll(src, "$HANDLER_NAME", title)
	src = strings.ReplaceAll(src, "$NAME", wsName)
	if err := copyDataToFile([]byte(src), fileName); err != nil {
		return err
	}

	color.Yellow("  - Created handlers/ws_%s.go", wsName)

	if err := wireWebsocketRoute(b.RootPath+"/routes.go", title, wsName); err != nil {
		return err
	}
	color.Yellow("  - Mounted /ws/%s in routes.go", wsName)

	// The handler imports regius/ws, which pulls gorilla/websocket into
	// the app's module graph: tidy so go.mod/go.sum carry it. Best-effort
	// (templ-generate precedent): unusual setups get a warning and a
	// manual instruction instead of a failed generation.
	if err := ensureWebsocketDep(); err != nil {
		color.Yellow("  ! go mod tidy failed; run it manually so gorilla/websocket lands in go.mod: %v", err)
	}

	color.Yellow("  - Next: dispatch on the event name in the read loop; broadcast to every socket with handlers.WSBroadcast%s(h.App, event, payload)", title)
	color.Yellow("  - Try it: websocat ws://localhost:PORT/ws/%s", wsName)

	return nil
}

// ensureWebsocketDep runs `go mod tidy` in the app root so the app's
// go.mod/go.sum pick up gorilla/websocket (a direct dependency through
// regius/ws). No-op when the app has no go.mod (not a module) or the
// dependency is already recorded.
func ensureWebsocketDep() error {
	goModPath := filepath.Join(b.RootPath, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return nil
	}
	if strings.Contains(string(data), "github.com/gorilla/websocket") {
		return nil
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = b.RootPath
	return runGoCmd(cmd, "go mod tidy")
}

// wireWebsocketRoute inserts the a.get("/ws/<name>", ...) mount into
// routes.go at the "// add any route here" marker (fallback: right after
// the home route, for apps generated before the marker existed).
// Idempotent per endpoint name.
func wireWebsocketRoute(routesPath, title, wsName string) error {
	block := fmt.Sprintf("	a.get(\"/ws/%[2]s\", a.Handlers.WS%[1]s)\n", title, wsName)

	return insertAtMarker(routesPath, "// add any route here", block,
		fmt.Sprintf("a.get(\"/ws/%s\"", wsName), func(content string) (string, bool) {
			return insertAfterLine(content, `a.get("/", a.Handlers.Home)`, block)
		})
}
