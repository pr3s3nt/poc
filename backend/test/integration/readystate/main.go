// Command readystate prepares a local fake-adapter JSON state file for a
// Playwright/video run: it writes explicit READY KUBECONFIG Connection
// metadata that references a synthetic scoped credential. The product seed
// never produces a READY Kubernetes record without an uploaded credential, and
// fake adapters never resolve it, so local fake workflows opt in here.
//
// The credential lives in a throwaway in-memory store of this process; it is
// not a real kubeconfig and no cluster is contacted. Existing keys are kept.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/adapters/store"
	ct "orchestrator/internal/application/connection/connectiontest"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
)

type connections []string

func (c *connections) String() string     { return strings.Join(*c, ",") }
func (c *connections) Set(v string) error { *c = append(*c, v); return nil }

func main() {
	defaults := seed.Defaults()
	statePath := flag.String("state", "", "JSON state file to create or extend (required)")
	organization := flag.String("organization", defaults.OrganizationKey, "Organization key")
	var extra connections
	flag.Var(&extra, "connection", "additional READY connection as key=kubeContext:cluster (repeatable)")
	flag.Parse()
	if *statePath == "" {
		fmt.Fprintln(os.Stderr, "readystate: -state is required")
		os.Exit(2)
	}
	st, err := store.NewWithSnapshot(*statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "readystate:", err)
		os.Exit(1)
	}
	ctx := context.Background()
	// The current snapshot format requires the owning Organization first; the
	// product seed at backend start keeps an existing one.
	if _, err := st.GetOrganization(ctx, *organization); errors.Is(err, persistence.ErrNotFound) {
		if err := st.SaveOrganization(ctx, application.Organization{ID: ids.New(), Key: *organization, Name: "Acme"}); err != nil {
			fmt.Fprintln(os.Stderr, "readystate:", err)
			os.Exit(1)
		}
	} else if err != nil {
		fmt.Fprintln(os.Stderr, "readystate:", err)
		os.Exit(1)
	}
	creds := credentialmemory.New()
	prepare := func(key, kubeContext, cluster string) error {
		if _, err := st.GetConnection(ctx, *organization, key); err == nil {
			return nil
		} else if !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
		return ct.PrepareReady(ctx, st, creds, *organization, key, kubeContext, cluster)
	}
	if err := prepare(defaults.ConnectionKey, defaults.KubeContext, defaults.ClusterName); err != nil {
		fmt.Fprintln(os.Stderr, "readystate:", err)
		os.Exit(1)
	}
	for _, spec := range extra {
		key, rest, ok := strings.Cut(spec, "=")
		kubeContext, cluster, ok2 := strings.Cut(rest, ":")
		if !ok || !ok2 || key == "" || kubeContext == "" || cluster == "" {
			fmt.Fprintf(os.Stderr, "readystate: invalid -connection %q, want key=kubeContext:cluster\n", spec)
			os.Exit(2)
		}
		if err := prepare(key, kubeContext, cluster); err != nil {
			fmt.Fprintln(os.Stderr, "readystate:", err)
			os.Exit(1)
		}
	}
}
