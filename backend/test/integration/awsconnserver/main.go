// Command awsconnserver is a test-only launcher of the orchestrator for the
// UC-04 AWS Connection onboarding browser scenario. It wires the PUBLIC
// product adapters (PostgreSQL Console store, Vault KV v2 credential store,
// the real read-only awsaccess verifier with SigV4 signing) and changes only
// the AWS transport: the verifier's HTTP endpoints point at an in-process
// deterministic FAKE AWS (awsfake) on a local port. The fake is selected by
// access key ID:
//
//	AKIAFAKESUCCESS00001   every check passes
//	AKIAFAKEREJECTED0001   AWS rejects the key
//	AKIAFAKENOPERMS00001   identity and regions pass, capabilities are denied
//	AKIAFAKENOSIMULATE01   the principal may not simulate its policies
//
// and, for regions, ap-east-1 is "disabled". Nothing here reaches AWS, and a
// passing scenario is NOT evidence that real AWS accepts a key (V2-A01/A02/
// A03 need a real run). Workload executors stay fake. On SIGINT/SIGTERM it
// writes the list of AWS calls it served (service and action only, never
// headers or bodies) to -aws-calls-file.
//
// "awsconnserver provision" and "awsconnserver drop" create and remove one
// disposable database with its roles on a PostgreSQL server the task owns.
package main

import (
	"context"
	"encoding/json"
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

	"orchestrator/internal/adapters/awsaccess"
	"orchestrator/internal/adapters/awsaccess/awsfake"
	"orchestrator/internal/adapters/vault"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/seed"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "provision":
			os.Exit(provision(os.Args[2:]))
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
	profile := flag.String("profile", "test", "seed profile")
	runID := flag.String("run-id", "", "run id")
	callsFile := flag.String("aws-calls-file", "", "file written on shutdown with the fake AWS calls (service and action only)")
	fakeKube := flag.Bool("fake-kubernetes", false, "verify Kubernetes kubeconfigs with the deterministic FAKE verifier (V01 account scenario)")
	journalFile := flag.String("credential-journal", "", "append-only file that records every credential object this process writes or deletes (exact run ownership)")
	kubeCallsFile := flag.String("kube-calls-file", "", "file written on shutdown with the fake Kubernetes verification endpoints")
	scoreK8s := flag.String("score-k8s", "", "optional path of the pinned score-k8s renderer; with it the Console lists its render bundle (the V02 rendering-template steps need one). Nothing is rendered or deployed")
	catalog := flag.String("catalog", "", `seed catalog: "" (walking-skeleton catalog) or "none" (no Types, Definitions or Connections: the V02 Definition scenario authors them)`)
	flag.Parse()
	if *databaseURLFile == "" || *vaultAddress == "" || *vaultTokenFile == "" {
		log.Print("awsconnserver: -database-url-file, -connection-vault-address and -connection-vault-token-file are required")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*databaseURLFile)
	if err != nil {
		log.Print("awsconnserver: read database URL file")
		os.Exit(1)
	}

	fake := awsfake.New()
	fake.Accept = func(keyID string) bool { return strings.HasPrefix(keyID, "AKIAFAKE") }
	fake.Rules = map[string]awsfake.Rule{
		"AKIAFAKEREJECTED0001": awsfake.RuleReject,
		"AKIAFAKENOPERMS00001": awsfake.RuleNoCapabilities,
		"AKIAFAKENOSIMULATE01": awsfake.RuleNoSimulate,
	}
	fake.Disabled["ap-east-1"] = true
	// V01 accounts: a second key of the same account (rotation) and a key of
	// another account (redirection must be refused).
	fake.KeyAccount = map[string]string{"AKIAFAKEOTHERACCT001": "210987654321"}
	fakeServer := fake.Start()
	defer fakeServer.Close()
	client := &awsaccess.Client{
		HTTP:     fakeServer.Client(),
		STSURL:   func(string) string { return fakeServer.URL + "/" },
		EKSURL:   func(string) string { return fakeServer.URL },
		QueryURL: func(string, string) string { return fakeServer.URL + "/" },
		IAMURL:   func() string { return fakeServer.URL + "/" },
	}

	opts.Profile, opts.RunID = *profile, *runID
	switch *catalog {
	case "":
	case "none":
		opts.Catalog = seed.CatalogNone
	default:
		log.Print("awsconnserver: -catalog must be empty or none")
		os.Exit(2)
	}
	kubeVerifier := &fakeKubernetes{}
	buildOptions := bootstrap.Options{
		Seed: opts, UIDir: *uiDir, Adapters: bootstrap.AdapterFake, DatabaseURL: strings.TrimSpace(string(raw)),
		ConnectionCredentialStore: "vault", ConnectionVaultAddress: *vaultAddress, ConnectionVaultTokenFile: *vaultTokenFile, ConnectionVaultMount: *vaultMount,
		AWSVerifierOverride: awsaccess.Verifier{Client: client},
		ScoreK8sPath:        *scoreK8s,
	}
	if *fakeKube {
		buildOptions.KubeconfigVerifierOverride = kubeVerifier
	}
	if *journalFile != "" {
		token, err := os.ReadFile(*vaultTokenFile)
		if err != nil {
			log.Print("awsconnserver: read Vault token file")
			os.Exit(1)
		}
		inner, err := vault.NewConnectionCredentials(*vaultAddress, strings.TrimSpace(string(token)), *vaultMount, nil)
		if err != nil {
			log.Print("awsconnserver: Vault credential store")
			os.Exit(1)
		}
		buildOptions.ConnectionCredentialStore, buildOptions.ConnectionCredentialsOverride = "", &journaledCredentials{inner: inner, path: *journalFile}
	}
	app, err := bootstrap.Build(context.Background(), buildOptions)
	if err != nil {
		log.Printf("awsconnserver: startup failed: %v", err)
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Printf("awsconnserver: listen: %v", err)
		os.Exit(1)
	}
	if *addrFile != "" {
		if err := os.WriteFile(*addrFile, []byte(listener.Addr().String()), 0o600); err != nil {
			log.Printf("awsconnserver: write addr file: %v", err)
			os.Exit(1)
		}
	}
	fmt.Printf("awsconnserver listening on http://%s (FAKE AWS transport)\n", listener.Addr())
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
		log.Printf("awsconnserver: %v", err)
		os.Exit(1)
	}
	if *kubeCallsFile != "" {
		encoded, _ := json.MarshalIndent(map[string]any{"fake": true, "calls": kubeVerifier.recorded()}, "", "  ")
		_ = os.WriteFile(*kubeCallsFile, encoded, 0o600)
	}
	if *callsFile != "" {
		calls := []string{}
		for _, call := range fake.Calls() {
			// "service METHOD Action path": the action (or path for EKS) only.
			fields := strings.Fields(call)
			calls = append(calls, strings.Join(fields[:min(3, len(fields))], " "))
		}
		encoded, _ := json.MarshalIndent(map[string]any{"fake": true, "calls": calls}, "", "  ")
		_ = os.WriteFile(*callsFile, encoded, 0o600)
	}
}
