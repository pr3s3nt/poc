// Package terraform holds the Terraform adapters of the orchestrator: the
// trusted packaged modules shipped with the binary (verified against the
// platform release manifest by the registry), the module registry, the CLI
// runner, the provider binder, the readiness check and the recovery store.
// The v2 executor itself is application/tfexec; there is no embedded
// executor that runs module code with local state any more.
package terraform

import (
	"embed"
	"io/fs"
)

//go:embed modules
var moduleFS embed.FS

// PackagedModules returns the trusted packaged module files shipped with the
// binary, rooted at the module directories ("vpc", "eks", "aurora", "elasticache", "k8s-namespace", "redis-k8s", "postgres-k8s"). The
// module registry verifies them against the release manifest digests.
func PackagedModules() fs.FS {
	sub, err := fs.Sub(moduleFS, "modules")
	if err != nil {
		panic(err)
	}
	return sub
}
