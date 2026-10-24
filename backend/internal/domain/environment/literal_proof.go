package environment

import (
	"crypto/sha256"
	"encoding/hex"
)

// LiteralProofs is the private, server-owned provenance of ordinary string
// container variables (UC-16 BR-23). It maps workload -> container -> variable
// name to a digest of the exact accepted value, and is stored beside one
// immutable Deployment Set, never inside its Document, a Score or a Delta.
//
// A literal in a deployed module is readable by an editor only when the same
// set proves it. Historical and direct-API literals have no proof and stay
// unreadable; a missing proof means "unknown", never "safe".
type LiteralProofs map[string]map[string]map[string]string

// LiteralDigest identifies the exact accepted value of one variable. It binds
// the workload, container and name, so a proof cannot be moved to another
// identity, and it is stored next to the value that the Document already holds.
func LiteralDigest(workload, container, name, value string) string {
	sum := sha256.New()
	for _, part := range []string{workload, container, name, value} {
		sum.Write([]byte{byte(len(part) >> 24), byte(len(part) >> 16), byte(len(part) >> 8), byte(len(part))})
		sum.Write([]byte(part))
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// Proves reports whether value is the accepted literal of the variable.
func (p LiteralProofs) Proves(workload, container, name, value string) bool {
	digest, ok := p[workload][container][name]
	return ok && digest == LiteralDigest(workload, container, name, value)
}

// Add records the proof of one accepted value, allocating as needed.
func (p LiteralProofs) Add(workload, container, name, value string) {
	if p[workload] == nil {
		p[workload] = map[string]map[string]string{}
	}
	if p[workload][container] == nil {
		p[workload][container] = map[string]string{}
	}
	p[workload][container][name] = LiteralDigest(workload, container, name, value)
}

// Empty reports whether no proof exists.
func (p LiteralProofs) Empty() bool {
	for _, containers := range p {
		for _, names := range containers {
			if len(names) > 0 {
				return false
			}
		}
	}
	return true
}

// Prover answers whether a deployed literal of one workload is proven.
type Prover func(container, name, value string) bool

// ProverFor returns the prover of one workload; a nil proof set proves nothing.
func (p LiteralProofs) ProverFor(workload string) Prover {
	return func(container, name, value string) bool { return p.Proves(workload, container, name, value) }
}
