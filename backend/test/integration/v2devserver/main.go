// Command v2devserver is a DEVELOPMENT launcher for the real-runner Playwright
// scenario script. It serves the product with a catalog-free seed so every
// Connection, Type, Definition, pin, mapping and template is authored through
// the Web Console, over the REAL PostgreSQL store, Vault Connection store,
// verified module registry and pinned score-k8s. Executors are FAKE and the
// kubeconfig verifier accepts any well-formed single-context document: nothing
// is provisioned or deployed, so a pass is NOT evidence of any V2 scenario.
// The real profile (acceptance-v2) refuses these seams.
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

	"orchestrator/internal/application/connection"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/seed"
)

type acceptingVerifier struct{}

func (acceptingVerifier) VerifyKubeconfig(_ context.Context, s connection.SelectedKubeconfig) (connection.KubernetesVerification, error) {
	return connection.KubernetesVerification{Endpoint: s.Context.Endpoint, Version: "v0.0-dev"}, nil
}

func main() {
	opts := seed.Defaults()
	addrFile := flag.String("addr-file", "", "file that receives the resolved listen address")
	uiDir := flag.String("ui-dir", "../frontend/dist", "Web Console production bundle")
	databaseURLFile := flag.String("database-url-file", "", "owner-only file with the PostgreSQL Console login URL")
	vaultAddress := flag.String("connection-vault-address", "", "Vault address")
	vaultTokenFile := flag.String("connection-vault-token-file", "", "scoped Vault token file")
	scoreK8s := flag.String("score-k8s", "", "pinned score-k8s binary")
	namespace := flag.String("namespace", opts.NamespaceIdentity, "namespace identity")
	flag.Parse()
	raw, err := os.ReadFile(*databaseURLFile)
	if err != nil || *vaultAddress == "" || *vaultTokenFile == "" || *scoreK8s == "" {
		log.Print("v2devserver: -database-url-file, -connection-vault-address, -connection-vault-token-file and -score-k8s are required")
		os.Exit(2)
	}
	opts.Profile, opts.Catalog, opts.NamespaceIdentity = "test", seed.CatalogNone, *namespace
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{
		Seed: opts, UIDir: *uiDir, Adapters: bootstrap.AdapterFake, DatabaseURL: strings.TrimSpace(string(raw)),
		ConnectionCredentialStore: "vault", ConnectionVaultAddress: *vaultAddress, ConnectionVaultTokenFile: *vaultTokenFile, ConnectionVaultMount: "secret",
		ScoreK8sPath: *scoreK8s, RequireScoreRenderer: true, KubeconfigVerifierOverride: acceptingVerifier{},
	})
	if err != nil {
		log.Printf("v2devserver: startup failed: %v", err)
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(1)
	}
	if *addrFile != "" {
		_ = os.WriteFile(*addrFile, []byte(listener.Addr().String()), 0o600)
	}
	fmt.Printf("v2devserver listening on http://%s (FAKE executors; development only)\n", listener.Addr())
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
		os.Exit(1)
	}
	app.Close()
}
