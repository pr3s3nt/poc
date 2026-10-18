package planning

import (
	"errors"
	"fmt"
	"sort"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
)

// ErrAccountConflict reports a unique winning Definition whose Driver Account
// is missing, foreign, not READY or of the wrong kind. The winner is never
// replaced by another candidate (UC-03 V02-02).
var ErrAccountConflict = errors.New("planning: the winning definition's driver account is unavailable")

var (
	errNoMatch   = errors.New("no match")
	errAmbiguous = errors.New("ambiguous match")
)

// matchError keeps the established message while exposing the outcome kind.
type matchError struct {
	kind error
	msg  string
}

func (e *matchError) Error() string        { return e.msg }
func (e *matchError) Is(target error) bool { return target == e.kind }

// definitionMatcher selects the most specific Resource Definition for each
// resource node (UC-03 BR-02 and BR-06, UC-06 MS-07).
type definitionMatcher struct {
	ctx      Context
	defs     map[string]resource.Definition
	list     []resource.Definition
	accounts map[string]application.Connection
}

func newDefinitionMatcher(ctx Context, catalog Catalog) (*definitionMatcher, error) {
	m := &definitionMatcher{ctx: ctx, defs: map[string]resource.Definition{}, accounts: catalog.Accounts}
	var defs []resource.Definition
	for _, d := range catalog.Definitions {
		if d.Key == BuiltinClusterKey {
			return nil, ErrReservedDefinition
		}
		defs = append(defs, d)
	}
	resource.SortDefinitions(defs)
	for _, d := range defs {
		if err := d.Validate(); err != nil {
			return nil, err
		}
		if _, ok := catalog.Types[d.ResourceTypeKey]; !ok {
			return nil, fmt.Errorf("planning: definition %q references unknown resource type %q", d.Key, d.ResourceTypeKey)
		}
		if _, exists := m.defs[d.Key]; exists {
			return nil, fmt.Errorf("planning: duplicate resource definition key %q", d.Key)
		}
		m.defs[d.Key] = d
	}
	m.list = defs
	return m, nil
}

func (m *definitionMatcher) definition(key string) (resource.Definition, bool) {
	d, ok := m.defs[key]
	return d, ok
}

// match returns the winning definition for one node.
func (m *definitionMatcher) match(node *Node) (Match, error) {
	ctx := resource.MatchContext{
		EnvironmentType: m.ctx.Env.Type,
		ApplicationID:   m.ctx.App.Key,
		EnvironmentID:   m.ctx.Env.Key,
		ResourceID:      descriptorID(node.Descriptor),
		Class:           node.Class,
	}

	ranking := resource.Rank(m.list, node.ResourceType, ctx, nil)
	switch len(ranking.Top) {
	case 0:
		return Match{}, &matchError{kind: errNoMatch, msg: fmt.Sprintf("planning: no resource definition matches %s (type %q, class %q)",
			node.Descriptor, node.ResourceType, node.Class)}
	case 1:
		win := ranking.Top[0]
		match := Match{
			Descriptor:        node.Descriptor,
			DefinitionKey:     win.Definition.Key,
			DriverType:        win.Definition.DriverType,
			DefinitionVersion: win.Definition.Version,
			DriverAccount:     win.Definition.DriverAccount,
			Specificity:       win.Score,
			Criterion:         win.Criterion,
		}
		if win.Definition.DriverType == resource.DriverExistingCluster {
			if err := m.pinClusterAccount(node, win.Definition, &match); err != nil {
				return Match{}, err
			}
		}
		return match, nil
	default:
		keys := make([]string, 0, len(ranking.Top))
		for _, c := range ranking.Top {
			keys = append(keys, c.Definition.Key)
		}
		sort.Strings(keys)
		return Match{}, &matchError{kind: errAmbiguous, msg: fmt.Sprintf("planning: ambiguous match for %s between %v", node.Descriptor, keys)}
	}
}

// pinClusterAccount checks the unique winning existing-cluster Definition's
// own Internal Driver Account and pins its version (ADR-016). The winner is
// never replaced by another candidate and no Environment or host account is a
// fallback.
func (m *definitionMatcher) pinClusterAccount(node *Node, def resource.Definition, match *Match) error {
	if !m.ctx.DefinitionPlaced() && def.DriverAccount == "" {
		return nil // planner-reference fixtures only
	}
	if node.ResourceType != TypeCluster {
		return fmt.Errorf("%w: existing-cluster definition %q matched %s which is not a Kubernetes cluster", ErrAccountConflict, def.Key, node.Descriptor)
	}
	account, ok := m.accounts[def.DriverAccount]
	if def.DriverAccount == "" || !ok || account.OrganizationKey != m.ctx.OrganizationKey ||
		account.Kind != application.ConnectionKubernetes || account.Status != application.ConnectionReady {
		return fmt.Errorf("%w: definition %q needs a READY Internal driver account of this organization", ErrAccountConflict, def.Key)
	}
	match.AccountVersion = account.Version
	return nil
}
