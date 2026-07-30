package main

import (
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/hbarral/regius"

	"regius-app/data"
	"regius-app/handlers"
	"regius-app/middleware"
)

type application struct {
	App        *regius.Regius
	Handlers   *handlers.Handlers
	Models     data.Models
	Middleware *middleware.Middleware
	wg         sync.WaitGroup
	done       chan struct{}
}

func main() {
	r := initApplication()
	r.done = make(chan struct{})
	go r.listenForShutdown()
	go r.startSSEHeartbeat()
	err := r.App.ListenAndServe()
	r.App.ErrorLog.Println(err)
}

func (a *application) shutdown() {
	// put any clean up tasks here
	close(a.done)

	// block until the WaitGroup is empty
	a.wg.Wait()
}

func (a *application) listenForShutdown() {
	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, syscall.SIGINT, syscall.SIGTERM)
	s := <-osSignals
	a.App.InfoLog.Println("Received signal:", s)
	a.shutdown()
	os.Exit(0)
}

func (a *application) startSSEHeartbeat() {
	if v := os.Getenv("SSE_DEMO_HEARTBEAT"); v != "" {
		if disabled, err := strconv.ParseBool(v); err == nil && disabled {
			return
		}
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := a.App.SSEBroadcastJSON("heartbeat", map[string]string{
				"time": time.Now().Format(time.RFC3339),
			})
			if err != nil {
				a.App.ErrorLog.Println("sse heartbeat error:", err)
			}
		case <-a.done:
			return
		}
	}
}
