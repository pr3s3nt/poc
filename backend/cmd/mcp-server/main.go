// Command mcp-server exposes orchestrator API operations to MCP clients.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"orchestrator/internal/mcpbridge"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	config := mcpbridge.Config{
		APIURL:   environment("ORCHESTRATOR_API_URL", "http://127.0.0.1:8080"),
		Username: os.Getenv("ORCHESTRATOR_USERNAME"),
		Password: os.Getenv("ORCHESTRATOR_PASSWORD"),
	}
	if allowed := os.Getenv("MCP_ALLOWED_ENVIRONMENTS"); allowed != "" {
		config.AllowedEnvironments = strings.Split(allowed, ",")
	}
	var audit *os.File
	if path := os.Getenv("MCP_AUDIT_PATH"); path != "" {
		var err error
		audit, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer audit.Close()
		config.Audit = audit
	}
	handler, err := mcpbridge.NewHandler(config)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              environment("MCP_LISTEN_ADDR", "127.0.0.1:8090"),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func environment(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
