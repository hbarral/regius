package regius

import (
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

	go r.listenRPC()

	r.InfoLog.Printf("Listening on port %s", os.Getenv("PORT"))

	return srv.ListenAndServe()
}
