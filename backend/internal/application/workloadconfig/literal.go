package workloadconfig

import (
	"strings"

	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/placeholder"
)

// hasUnescapedPlaceholder reports a ${...} that the Score expander would
// resolve. An escaped $${...} is text.
func hasUnescapedPlaceholder(value string) bool {
	for _, match := range placeholder.Pattern.FindAllString(value, -1) {
		if !strings.HasPrefix(match, "$$") {
			return true
		}
	}
	return false
}

// isOrdinaryValue reports a container variable that is plain text under the
// Score syntax: any string, including empty, whitespace and escaped $${...},
// without a resolvable placeholder. NUL is rejected by validateBindings.
func isOrdinaryValue(value string) bool { return !hasUnescapedPlaceholder(value) }

// ProveLiterals builds the provenance of the Deployment Set a successful
// workload deploy produces (UC-16 BR-23). base is the proof of the set the
// deploy started from; document is the candidate content. A literal is proven
// when
//
//   - it belongs to accepted (the workload the UC-16 pending batch deployed
//     from a validated draft) and is an ordinary value, or
//   - the base set already proved the identical value for the same workload,
//     container and name (carried through unrelated deploys, restart and
//     rename-free redeploys).
//
// Any other literal, including one a direct Score API changed, is left
// unproven and so stays unreadable. Nothing here reads or stores a secret: a
// reference binding is never a literal.
func ProveLiterals(base environment.LiteralProofs, document environment.Document, accepted string) environment.LiteralProofs {
	out := environment.LiteralProofs{}
	for workload, module := range document.Modules {
		for container, spec := range module.Spec.Containers {
			for name, value := range spec.Variables {
				if hasUnescapedPlaceholder(value) {
					continue
				}
				if (workload == accepted && accepted != "") || base.Proves(workload, container, name, value) {
					out.Add(workload, container, name, value)
				}
			}
		}
	}
	if out.Empty() {
		return nil
	}
	return out
}
