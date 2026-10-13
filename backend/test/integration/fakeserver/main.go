// Command fakeserver is a test-only launcher of the orchestrator with fake
// adapters, in-memory state and the explicit non-durable memory credential
// store. After the product seed it marks the seeded cluster READY with a
// synthetic credential in that same injected store (bootstraptest), because the
// product seed is VERIFYING without credentials and the memory store cannot
// be combined with -state. No product binary has a READY shortcut; this is
// setup only. Playwright scenarios still do their product assertions via UI/API.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"orchestrator/internal/bootstrap"
	"orchestrator/internal/bootstrap/bootstraptest"
	"orchestrator/internal/seed"
)

func main() {
	opts := seed.Defaults()
	addr := flag.String("addr", "127.0.0.1:0", "listen address")
	addrFile := flag.String("addr-file", "", "file that receives the resolved listen address")
	uiDir := flag.String("ui-dir", "../frontend/dist", "Web Console production bundle")
	profile := flag.String("profile", opts.Profile, "seed profile")
	runID := flag.String("run-id", "", "run id")
	region := flag.String("region", "", "AWS region of the fake cloud Application")
	accountID := flag.String("account-id", "", "AWS account id of the fake cloud Application")
	scoreK8s := flag.String("score-k8s", "", "score-k8s binary or stand-in")
	requireScore := flag.Bool("require-score-renderer", true, "product renderer policy: every Workload needs a score-k8s Definition and bundle, no native fallback")
	flag.Parse()
	opts.Profile, opts.RunID, opts.Region, opts.AccountID = *profile, *runID, *region, *accountID
	app, err := bootstraptest.Build(context.Background(), bootstrap.Options{
		Seed: opts, UIDir: *uiDir, Adapters: bootstrap.AdapterFake, ScoreK8sPath: *scoreK8s, RequireScoreRenderer: *requireScore, ConnectionCredentialStore: "memory",
	})
	if err != nil {
		log.Printf("fakeserver: startup failed: %v", err)
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Printf("fakeserver: listen: %v", err)
		os.Exit(1)
	}
	if *addrFile != "" {
		if err := os.WriteFile(*addrFile, []byte(listener.Addr().String()), 0o600); err != nil {
			log.Printf("fakeserver: write addr file: %v", err)
			os.Exit(1)
		}
	}
	fmt.Printf("fakeserver listening on http://%s\n", listener.Addr())
	server := &http.Server{Handler: app.Server, ReadHeaderTimeout: 10 * time.Second}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Printf("fakeserver: %v", err)
		os.Exit(1)
	}
}
