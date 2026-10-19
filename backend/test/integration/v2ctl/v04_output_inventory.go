package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"orchestrator/test/integration/v03run"
	"os"
	"strings"
	"time"
)

type v04OutputBaseline struct {
	Version int      `json:"version"`
	RunID   string   `json:"runId"`
	Address string   `json:"address"`
	Mount   string   `json:"mount"`
	Refs    []string `json:"refs"`
}

func v04OutputInventory(args []string) int {
	fs := flag.NewFlagSet("v04-output-inventory", flag.ExitOnError)
	mode := fs.String("mode", "", "baseline, check or verify")
	address, mount := fs.String("vault-addr", "", ""), fs.String("mount", "secret", "")
	tokenFile, baselineFile := fs.String("token-file", "", ""), fs.String("baseline-file", "", "")
	runID, refsFile := fs.String("run-id", "", ""), fs.String("refs-file", "", "")
	_ = fs.Parse(args)
	if (*mode != "baseline" && *mode != "check" && *mode != "verify") || *runID == "" || *baselineFile == "" {
		return fail("output inventory needs mode, exact run and baseline handle")
	}
	token, err := os.ReadFile(*tokenFile)
	if err != nil {
		return fail("output metadata token is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	live, err := (v03run.Vault{Address: *address, Mount: *mount, Token: strings.TrimSpace(string(token))}).ListOutputRefs(ctx)
	if err != nil {
		return fail("output metadata inventory is incomplete: %v", err)
	}
	if *mode == "baseline" {
		if _, err := os.Stat(*baselineFile); !os.IsNotExist(err) {
			return fail("refusing to replace output baseline")
		}
		if err := writeJSON(*baselineFile, v04OutputBaseline{Version: 1, RunID: *runID, Address: *address, Mount: *mount, Refs: live}); err != nil {
			return fail("private output baseline could not be written")
		}
		fmt.Printf("output metadata baseline recorded: %d protected objects\n", len(live))
		return 0
	}
	raw, err := os.ReadFile(*baselineFile)
	if err != nil {
		return fail("output baseline is missing; retain SQL and recovery")
	}
	var baseline v04OutputBaseline
	if json.Unmarshal(raw, &baseline) != nil || baseline.Version != 1 || baseline.RunID != *runID || baseline.Address != *address || baseline.Mount != *mount || baseline.Refs == nil {
		return fail("output baseline identity/shape differs; retain SQL and recovery")
	}
	raw, err = os.ReadFile(*refsFile)
	if err != nil {
		return fail("owned output reference inventory is missing")
	}
	var owned []string
	if json.Unmarshal(raw, &owned) != nil || owned == nil {
		return fail("owned output inventory is malformed")
	}
	if err := v03run.CheckOutputInventory(baseline.Refs, live, owned, *mode == "verify"); err != nil {
		return fail("%v", err)
	}
	fmt.Printf("output metadata inventory %s proven: %d protected objects, %d inventoried owned refs\n", *mode, len(baseline.Refs), len(owned))
	return 0
}
