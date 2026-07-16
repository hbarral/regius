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
		Handler:      r.Routes,
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

	r.InfoLog.Printf("Listening on port %s", os.Getenv("PORT"))

	return srv.ListenAndServe()
}
