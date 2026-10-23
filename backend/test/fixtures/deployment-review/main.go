// Command deployment-review serves a loopback-only, disposable browser fixture.
// All execution adapters and cluster credentials are synthetic. It is never a product entry point.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ui := flag.String("ui-dir", "", "built Console directory")
	cache := flag.String("cache-dir", "", "private disposable module cache outside Git")
	addrFile := flag.String("addr-file", "", "file for bound loopback URL")
	flag.Parse()
	if *ui == "" || *cache == "" || *addrFile == "" {
		log.Fatal("ui-dir, cache-dir and addr-file are required")
	}
	renderer, err := exec.LookPath("score-k8s")
	if err != nil {
		log.Fatal("installed selected renderer required")
	}
	opts := seedtest.Defaults()
	opts.Profile = "test"
	opts.Catalog = seed.CatalogTerraformK8s
	opts.Region = "us-east-1"
	opts.AccountID = "000000000000"
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{Seed: opts, Adapters: bootstrap.AdapterFake, UIDir: *ui, ScoreK8sPath: renderer, RequireScoreRenderer: true, TerraformModuleCacheDir: *cache, ConnectionCredentialsOverride: credentialmemory.New()})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Handler: app.Server, ReadHeaderTimeout: 10 * time.Second}
	if err := os.WriteFile(*addrFile, []byte("http://"+listener.Addr().String()), 0600); err != nil {
		log.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-ctx.Done()
		timeout, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = server.Shutdown(timeout)
	}()
	log.Print("Disposable browser fixture: real product HTTP, fake execution adapters; loopback only")
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
