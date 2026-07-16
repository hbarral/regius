package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

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
}

func main() {
	r := initApplication()
	go r.listenForShutdown()
	err := r.App.ListenAndServe()
	r.App.ErrorLog.Println(err)
}

func (a *application) shutdown() {
	// put any clean up tasks here

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
