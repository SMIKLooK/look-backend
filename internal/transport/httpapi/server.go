package httpapi

import (
	"context"
	"net/http"
	"time"
)

// Server — HTTP-сервер с настроенными таймаутами.
type Server struct {
	srv *http.Server
}

// NewServer создаёт сервер. WriteTimeout должен быть больше таймаута
// обращения к провайдерам (LOOK_PROVIDER_TIMEOUT).
func NewServer(addr string, handler http.Handler) *Server {
	return &Server{srv: &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}}
}

// ListenAndServe запускает приём соединений.
func (s *Server) ListenAndServe() error { return s.srv.ListenAndServe() }

// Shutdown корректно останавливает сервер, дождавшись активных запросов.
func (s *Server) Shutdown(ctx context.Context) error { return s.srv.Shutdown(ctx) }
