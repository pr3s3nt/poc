package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"orchestrator/test/integration/v2run"
)

func init() { extra["aws-ebs-clean"] = awsEBSClean }

func awsEBSClean(args []string) int {
	fs := flag.NewFlagSet("aws-ebs-clean", flag.ExitOnError)
	cfgPath, manifestPath := fs.String("config", "", ""), fs.String("manifest", "", "")
	_ = fs.Parse(args)
	c, cli, code := awsRunner(*cfgPath)
	if code != 0 {
		return code
	}
	m, err := v2run.LoadManifest(*manifestPath)
	if err != nil {
		return fail("the private AWS manifest cannot be read")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := v2run.CleanupOrphanEBS(ctx, cli, c, m); err != nil {
		return fail("CSI volume recovery held: %v", err)
	}
	fmt.Println("exact run-owned orphan CSI volumes reconciled; native layer verification still required")
	return 0
}
