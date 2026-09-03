package regius

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

func (r *Regius) ListenAndServe() error {
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", os.Getenv("PORT")),
		ErrorLog:     r.ErrorLog,
		Handler:      r.Handler(),
		IdleTimeout:  30 * time.Second,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 600 * time.Second,
	}

	if r.DB.Pool != nil {
		defer r.DB.Pool.Close()
	}

	if redisPool != nil {
		defer redisPool.Close()
	}

	if badgerConn != nil {
		defer badgerConn.Close()
	}

	rpcListener, err := NewRPCListener(os.Getenv("RPC_PORT"), r.InfoLog, r.ErrorLog)
	if err != nil {
		r.ErrorLog.Println(err)
	}
	if rpcListener != nil {
		rpcCtx, rpcCancel := context.WithCancel(context.Background())
		defer rpcCancel()
		defer rpcListener.Stop()
		go func() {
			if err := rpcListener.Start(rpcCtx); err != nil && !errors.Is(err, context.Canceled) {
				r.ErrorLog.Println(err)
			}
		}()
	}

	// Jobs workers and the scheduler run in this process only when enabled;
	// app code has already registered its handlers by the time the app
	// serves. When ListenAndServe returns, workers are drained (in-flight
	// attempts cancelled and their jobs requeued) within the graceful
	// timeout.
	if r.Jobs != nil && r.config.jobs.enabled {
		r.Jobs.Start(context.Background())
		defer func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), r.config.jobs.gracefulTimeout)
			defer cancel()
			if err := r.Jobs.Stop(stopCtx); err != nil {
				r.ErrorLog.Printf("jobs: %v", err)
			}
		}()
	}

	r.InfoLog.Printf("Listening on port %s", os.Getenv("PORT"))

	return srv.ListenAndServe()
}
