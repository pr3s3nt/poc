// Command runidentity prints the deterministic identity of one target
// generation of an Environment: its physical namespace and the run id stamped
// on the objects a transition creates there. Verification scripts use it to
// prove that a namespace belongs to THIS run before deleting it, without
// matching on broad labels.
//
//	runidentity <application-key> <environment-key> <generation>
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"orchestrator/internal/application/transition"
	"orchestrator/internal/domain/environment"
)

var key = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

func main() {
	if len(os.Args) != 4 || !key.MatchString(os.Args[1]) || !key.MatchString(os.Args[2]) {
		fmt.Fprintln(os.Stderr, "usage: runidentity <application-key> <environment-key> <generation>")
		os.Exit(2)
	}
	generation, err := strconv.ParseInt(os.Args[3], 10, 64)
	if err != nil || generation < 0 {
		fmt.Fprintln(os.Stderr, "generation must be a non-negative integer")
		os.Exit(2)
	}
	app, env := os.Args[1], os.Args[2]
	identity := "app-" + app + "-" + env
	out := map[string]any{"namespace": environment.NamespaceFor(identity, generation)}
	if generation > 0 {
		out["runId"] = transition.RunIDFor(app, env, generation)
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
}
