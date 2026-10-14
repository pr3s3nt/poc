// Command v2supervisor is the independent cleanup supervisor of the real
// Terraform v2 acceptance runner. It must be started with
// v2run.StartDetached (own session and process group) BEFORE the first create.
//
//	v2supervisor -spec PRIVATE_SPEC.json
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"orchestrator/test/integration/v2run"
)

func main() {
	spec := flag.String("spec", "", "private 0600 supervisor spec")
	flag.Parse()
	s, err := v2run.LoadSpec(*spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "v2supervisor:", err)
		os.Exit(v2run.ExitBadSpecCode)
	}
	os.Exit((&v2run.Supervisor{Spec: s}).Run(context.Background()))
}
