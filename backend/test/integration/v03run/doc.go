// Package v03run holds the read-only verification and the guarded safety-net
// cleanup of the V03 internal-frontend acceptance runner. It reuses the M04
// Kubectl and Check types of v2run and never starts, builds or configures the
// product. The host kubeconfig handle it receives is used only by the runner
// and the safety net for read-only assertions and for deleting the exact
// namespaces the run owns (name, UID and orchestrator-managed label proven).
package v03run
