package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"Lib/utils/toolhub/internal/catalog"
	"Lib/utils/toolhub/internal/httpapi"
	"Lib/utils/toolhub/internal/supervisor"
)

const (
	defaultAddress = "127.0.0.1:17840"
	shutdownLimit  = 5 * time.Second
)

// Embedded assets keep the dashboard and default central catalog available
// when ToolHub is built as one binary. -catalog remains available for review or
// testing an edited registry without rebuilding.
//
//go:embed web/* catalog/tools.yaml
var assets embed.FS

// main owns only configuration, OS signals, and the HTTP server boundary.
func main() {
	address := flag.String("addr", defaultAddress, "ToolHub loopback listen address")
	catalogPath := flag.String("catalog", "", "optional central catalog path")
	repositoryRoot := flag.String("repo", "", "repository root; normally detected from the current directory")
	validateOnly := flag.Bool("validate", false, "validate the central catalog and exit")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if *validateOnly {
		if err := validateCatalog(*catalogPath, *repositoryRoot); err != nil {
			logger.Error("catalog validation failed", "error", err)
			os.Exit(1)
		}
		logger.Info("catalog is valid")
		return
	}
	if err := run(*address, *catalogPath, *repositoryRoot, logger); err != nil {
		logger.Error("ToolHub stopped", "error", err)
		os.Exit(1)
	}
}

// validateCatalog gives coding agents a side-effect-free registration check;
// it does not bind a port or start any managed command.
func validateCatalog(catalogPath, repositoryRoot string) error {
	root, err := resolveRepositoryRoot(repositoryRoot)
	if err != nil {
		return err
	}
	_, err = loadCatalog(catalogPath, root)
	return err
}

// run composes the catalog, process manager, and local web server. It does not
// stop managed tools when the browser closes because browser tabs have no
// reliable ownership signal; users stop each tool explicitly from the page.
func run(address, catalogPath, repositoryRoot string, logger *slog.Logger) error {
	if err := validateAddress(address); err != nil {
		return err
	}
	root, err := resolveRepositoryRoot(repositoryRoot)
	if err != nil {
		return err
	}
	registry, err := loadCatalog(catalogPath, root)
	if err != nil {
		return err
	}
	web, err := fs.Sub(assets, "web")
	if err != nil {
		return fmt.Errorf("load embedded web assets: %w", err)
	}
	return serve(address, registry, web, logger)
}

// serve starts background health observation and performs bounded HTTP
// shutdown. Managed child tools remain explicit user-controlled sessions.
func serve(address string, registry catalog.Registry, web fs.FS, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	manager := supervisor.NewManager(registry.Tools)
	manager.StartMonitor(ctx)
	server := &http.Server{
		Addr: address, Handler: httpapi.NewHandler(manager, web),
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	logger.Info("ToolHub is ready", "url", "http://"+address, "tools", len(registry.Tools))

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownLimit)
		defer cancel()
		return server.Shutdown(shutdownContext)
	}
}

// loadCatalog chooses an explicit development file or the reviewed embedded
// catalog while sharing the same strict decoder and validation path.
func loadCatalog(path, repositoryRoot string) (catalog.Registry, error) {
	var reader io.ReadCloser
	if path == "" {
		file, err := assets.Open("catalog/tools.yaml")
		if err != nil {
			return catalog.Registry{}, fmt.Errorf("open embedded catalog: %w", err)
		}
		reader = file
	} else {
		file, err := os.Open(path)
		if err != nil {
			return catalog.Registry{}, fmt.Errorf("open catalog %q: %w", path, err)
		}
		reader = file
	}
	defer reader.Close()
	return catalog.Load(reader, repositoryRoot)
}

// resolveRepositoryRoot supports execution from the repository root or any
// descendant, while an explicit flag covers installed binaries elsewhere.
func resolveRepositoryRoot(explicit string) (string, error) {
	if explicit != "" {
		absolute, err := filepath.Abs(explicit)
		if err != nil {
			return "", fmt.Errorf("resolve repository root: %w", err)
		}
		return absolute, nil
	}
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("read current directory: %w", err)
	}
	for {
		if isLibRepository(current) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("cannot find Lib repository root; pass -repo")
		}
		current = parent
	}
}

// isLibRepository checks content as well as filename to avoid selecting an
// unrelated parent go.mod in a nested workspace.
func isLibRepository(directory string) bool {
	content, err := os.ReadFile(filepath.Join(directory, "go.mod"))
	return err == nil && strings.Contains(string(content), "module Lib")
}

// validateAddress makes the local-only security promise non-configurable by
// accident. Remote access would require a separate authentication design.
func validateAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("ToolHub must listen on a loopback address")
	}
	return nil
}
