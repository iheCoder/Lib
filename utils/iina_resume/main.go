package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultAddress       = "127.0.0.1:17845"
	gracefulStopDeadline = 5 * time.Second
)

// main keeps process-level concerns at the edge. IINA inspection and HTTP
// behavior remain independently testable and do not depend on global state.
func main() {
	address := flag.String("addr", defaultAddress, "local address used by the web interface")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(*address, logger); err != nil {
		logger.Error("server stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}

// run owns the service lifecycle. launchd normally terminates user agents with
// SIGTERM, so a bounded shutdown prevents an in-flight API response from being
// cut in half during reinstall or logout.
func run(address string, logger *slog.Logger) error {
	application := newApplication(newCommandRunner(), logger)
	server := &http.Server{
		Addr:              address,
		Handler:           application.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	stopContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()

	logger.Info("IINA Resume is ready", "url", fmt.Sprintf("http://%s", address))
	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-stopContext.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), gracefulStopDeadline)
		defer cancel()
		return server.Shutdown(shutdownContext)
	}
}
