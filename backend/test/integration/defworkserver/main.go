// Command defworkserver is a test-only launcher of the orchestrator for the
// I04-07 Definition / Workload class / Preview browser scenario. It wires the
// PUBLIC product adapters that matter to the scenario: the PostgreSQL Console
// store, the Vault KV v2 Connection credential store, the verified Terraform
// module registry over the packaged release modules and the REAL pinned
// score-k8s renderer (required on both profiles, no native fallback). Workload
// and resource executors stay FAKE: nothing is provisioned or deployed, and a
// passing scenario is NOT live proof for V2-A01/A02/A03.
//
// "defworkserver provision" and "defworkserver drop" create and remove one
// disposable database with its roles on a PostgreSQL server the task owns.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/seed"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "provision":
			os.Exit(provision(os.Args[2:]))
		case "seed-progress":
			os.Exit(seedProgress(os.Args[2:]))
		case "drop":
			os.Exit(drop(os.Args[2:]))
		}
	}
	opts := seed.Defaults()
	addr := flag.String("addr", "127.0.0.1:0", "listen address")
	addrFile := flag.String("addr-file", "", "file that receives the resolved listen address")
	uiDir := flag.String("ui-dir", "../frontend/dist", "Web Console production bundle")
	databaseURLFile := flag.String("database-url-file", "", "owner-only file with the PostgreSQL Console login URL")
	vaultAddress := flag.String("connection-vault-address", "", "Vault address of the Connection credential store")
	vaultTokenFile := flag.String("connection-vault-token-file", "", "scoped Vault token file")
	vaultMount := flag.String("connection-vault-mount", "secret", "KV v2 mount")
	scoreK8s := flag.String("score-k8s", "", "path of the pinned score-k8s 0.15.0 renderer")
	profile := flag.String("profile", "test", "seed profile")
	runID := flag.String("run-id", "", "run id")
	flag.Parse()
	if *databaseURLFile == "" || *vaultAddress == "" || *vaultTokenFile == "" || *scoreK8s == "" {
		log.Print("defworkserver: -database-url-file, -connection-vault-address, -connection-vault-token-file and -score-k8s are required")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*databaseURLFile)
	if err != nil {
		log.Print("defworkserver: read database URL file")
		os.Exit(1)
	}
	opts.Profile, opts.RunID = *profile, *runID
	// bootstraptest.Build marks the seeded cluster READY with a synthetic
	// credential written to the Vault store: setup only, no product shortcut.
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{
		Seed: opts, UIDir: *uiDir, Adapters: bootstrap.AdapterFake, DatabaseURL: strings.TrimSpace(string(raw)),
		ConnectionCredentialStore: "vault", ConnectionVaultAddress: *vaultAddress, ConnectionVaultTokenFile: *vaultTokenFile, ConnectionVaultMount: *vaultMount,
		ScoreK8sPath: *scoreK8s, RequireScoreRenderer: true,
	})
	if err != nil {
		log.Printf("defworkserver: startup failed: %v", err)
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Printf("defworkserver: listen: %v", err)
		os.Exit(1)
	}
	if *addrFile != "" {
		if err := os.WriteFile(*addrFile, []byte(listener.Addr().String()), 0o600); err != nil {
			log.Printf("defworkserver: write addr file: %v", err)
			os.Exit(1)
		}
	}
	fmt.Printf("defworkserver listening on http://%s (FAKE executors, real score-k8s, PostgreSQL, Vault)\n", listener.Addr())
	server := &http.Server{Handler: app.Server, ReadHeaderTimeout: 10 * time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("defworkserver: %v", err)
		os.Exit(1)
	}
	app.Close()
}
