package server

import (
	"context"
	"expvar"
	"fmt"
	"net/http"
	"time"
)

type Server struct {
	srv         *http.Server
	mux         *http.ServeMux
	stopTimeout time.Duration
}

func New(ip string, port int, stopTimeout time.Duration) *Server {
	addr := fmt.Sprintf("%s:%d", ip, port)
	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	mux.HandleFunc("/healthz", ping)
	mux.Handle("/debug/vars", expvar.Handler())

	return &Server{
		srv:         srv,
		mux:         mux,
		stopTimeout: stopTimeout,
	}
}

func (s *Server) Run() error {
	return s.srv.ListenAndServe()
}

func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), s.stopTimeout)
	defer cancel()
	s.srv.Shutdown(ctx)
}

func (s *Server) String() string {
	return "http server"
}

func ping(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}
