package planning

import (
	"fmt"
	"sort"
	"strings"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning/placeholder"
	"orchestrator/internal/platform/canon"
)

type graphBuilder struct {
	ctx      Context
	catalog  Catalog
	order    []string
	nodes    map[string]*Node
	edges    []Edge
	edgeSeen map[string]bool
	expanded map[string]bool
}

func newGraphBuilder(ctx Context, catalog Catalog) *graphBuilder {
	return &graphBuilder{
		ctx:      ctx,
		catalog:  catalog,
		nodes:    map[string]*Node{},
		edgeSeen: map[string]bool{},
		expanded: map[string]bool{},
	}
}

// ensure creates the node for a descriptor once and records why it exists.
func (b *graphBuilder) ensure(d resource.Descriptor, kind NodeKind, origin Origin, params map[string]any) (*Node, error) {
	key := d.String()
	node, ok := b.nodes[key]
	if !ok {
		scope, err := ScopeFor(b.ctx, d)
		if err != nil {
			return nil, err
		}
		if params == nil {
			params = map[string]any{}
		}
		node = &Node{
			Descriptor:   key,
			Kind:         kind,
			ResourceType: d.Type,
			Class:        d.Class,
			Scope:        scope,
			Params:       params,
			Bindings:     map[string]string{},
		}
		b.nodes[key] = node
		b.order = append(b.order, key)
	} else if len(params) > 0 && len(node.Params) == 0 {
		node.Params = params
	}
	node.addOrigin(origin)
	return node, nil
}

func (n *Node) addOrigin(origin Origin) {
	if origin == "" {
		return
	}
	for _, existing := range n.Origins {
		if existing == origin {
			return
		}
	}
	n.Origins = append(n.Origins, origin)
	sort.Slice(n.Origins, func(i, j int) bool { return n.Origins[i] < n.Origins[j] })
}

func (b *graphBuilder) addEdge(consumer, provider, reason, path string) bool {
	if consumer == provider {
		return false
	}
	key := strings.Join([]string{consumer, provider, reason, path}, "\x00")
	if b.edgeSeen[key] {
		return false
	}
	b.edgeSeen[key] = true
	b.edges = append(b.edges, Edge{Consumer: consumer, Provider: provider, Reason: reason, Path: path})
	return true
}

func (b *graphBuilder) bind(consumer *Node, key, provider string) {
	if consumer.Bindings == nil {
		consumer.Bindings = map[string]string{}
	}
	consumer.Bindings[key] = provider
}

func (b *graphBuilder) descriptors() []string {
	out := append([]string(nil), b.order...)
	sort.Strings(out)
	return out
}

// consumersOf lists every node that already depends on the given provider.
func (b *graphBuilder) consumersOf(provider string) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range b.edges {
		if e.Provider == provider && !seen[e.Consumer] {
			seen[e.Consumer] = true
			out = append(out, e.Consumer)
		}
	}
	sort.Strings(out)
	return out
}

func (b *graphBuilder) graph() Graph {
	nodes := make([]Node, 0, len(b.nodes))
	for _, key := range b.descriptors() {
		nodes = append(nodes, *b.nodes[key])
	}
	edges := append([]Edge(nil), b.edges...)
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].Consumer != edges[j].Consumer {
			return edges[i].Consumer < edges[j].Consumer
		}
		if edges[i].Provider != edges[j].Provider {
			return edges[i].Provider < edges[j].Provider
		}
		return edges[i].Path < edges[j].Path
	})
	return Graph{Nodes: nodes, Edges: edges}
}

// enrichProfile adds the implicit infrastructure of the Execution Profile
// (UC-06 MS-05, VAR-01 and VAR-02) and returns the namespace node.
func (b *graphBuilder) enrichProfile() (*Node, error) {
	nsDescriptor, err := NamespaceDescriptor(b.ctx.App.Key, b.ctx.Env.Key)
	if err != nil {
		return nil, err
	}
	namespace, err := b.ensure(nsDescriptor, NodeResource, OriginProfile, nil)
	if err != nil {
		return nil, err
	}

	switch b.ctx.Env.Profile {
	case application.ProfileAWSEKS:
		vpcDescriptor, err := VPCDescriptor(b.ctx)
		if err != nil {
			return nil, err
		}
		eksDescriptor, err := EKSDescriptor(b.ctx)
		if err != nil {
			return nil, err
		}
		if _, err := b.ensure(vpcDescriptor, NodeResource, OriginProfile, nil); err != nil {
			return nil, err
		}
		if _, err := b.ensure(eksDescriptor, NodeResource, OriginProfile, nil); err != nil {
			return nil, err
		}
		b.addEdge(eksDescriptor.String(), vpcDescriptor.String(), ReasonProfile, "")
		b.addEdge(namespace.Descriptor, eksDescriptor.String(), ReasonProfile, "")
	case application.ProfileInternalK8s:
		clusterDescriptor, err := InternalClusterDescriptor(b.ctx.Env.ConnectionKey)
		if err != nil {
			return nil, err
		}
		if _, err := b.ensure(clusterDescriptor, NodeResource, OriginProfile, nil); err != nil {
			return nil, err
		}
		b.addEdge(namespace.Descriptor, clusterDescriptor.String(), ReasonProfile, "")
	default:
		return nil, fmt.Errorf("planning: unsupported execution profile %q", b.ctx.Env.Profile)
	}
	return namespace, nil
}

// buildFromSet derives nodes and edges from the Candidate Deployment Set.
func (b *graphBuilder) buildFromSet(set environment.Document, namespace *Node) error {
	workloadNodes := map[string]*Node{}
	privateNodes := map[string]map[string]*Node{}

	for _, id := range set.ModuleIDs() {
		module := set.Modules[id]
		wd, err := WorkloadDescriptor(id)
		if err != nil {
			return err
		}
		workload, err := b.ensure(wd, NodeWorkload, OriginWorkload, nil)
		if err != nil {
			return err
		}
		workload.WorkloadID = id
		workloadNodes[id] = workload
		privateNodes[id] = map[string]*Node{}

		b.addEdge(workload.Descriptor, namespace.Descriptor, ReasonProfile, "")
		b.bind(workload, "namespace", namespace.Descriptor)

		for _, name := range module.ExternalNames() {
			entry := module.Externals[name]
			pd, err := PrivateDescriptor(id, name, entry)
			if err != nil {
				return err
			}
			node, err := b.ensure(pd, NodeResource, OriginPrivate, copyParams(entry.Params))
			if err != nil {
				return err
			}
			privateNodes[id][name] = node
			b.addEdge(workload.Descriptor, node.Descriptor, ReasonPrivate,
				pointer("modules", id, "externals", name))
			b.bind(workload, placeholder.ScopeExternals+"."+name, node.Descriptor)
		}
	}

	sharedNodes := map[string]*Node{}
	for _, id := range set.SharedIDs() {
		entry := set.Shared[id]
		sd, err := SharedDescriptor(id, entry)
		if err != nil {
			return err
		}
		node, err := b.ensure(sd, NodeResource, OriginShared, copyParams(entry.Params))
		if err != nil {
			return err
		}
		sharedNodes[id] = node
	}

	resolve := func(ref placeholder.Ref, moduleID, path string) (*Node, error) {
		scope, name, found := strings.Cut(ref.Resource, ".")
		if !found {
			return nil, fmt.Errorf("planning: %s references %q which is not a Deployment Set placeholder", path, ref.Resource)
		}
		var node *Node
		switch scope {
		case placeholder.ScopeShared:
			node = sharedNodes[name]
		case placeholder.ScopeExternals:
			if moduleID != "" {
				node = privateNodes[moduleID][name]
			}
		default:
			return nil, fmt.Errorf("planning: %s references unknown scope %q", path, scope)
		}
		if node == nil {
			return nil, fmt.Errorf("planning: %s references unknown resource %q", path, ref.Resource)
		}
		if ref.OutputKey != "" {
			typ, ok := b.catalog.Types[node.ResourceType]
			if !ok {
				return nil, fmt.Errorf("planning: resource type %q is not registered", node.ResourceType)
			}
			if _, ok := typ.Output(ref.OutputKey); !ok {
				return nil, fmt.Errorf("planning: %s binds output %q which is not in the %q output contract",
					path, ref.OutputKey, typ.Key)
			}
		}
		return node, nil
	}

	// Workload placeholders read resource outputs from the module spec.
	for _, id := range set.ModuleIDs() {
		module := set.Modules[id]
		workload := workloadNodes[id]
		spec, err := canon.Map(module.Spec)
		if err != nil {
			return err
		}
		err = placeholder.WalkStrings(spec, pointer("modules", id, "spec"), func(path, value string) error {
			refs, err := placeholder.Refs(value)
			if err != nil {
				return fmt.Errorf("planning: %s: %w", path, err)
			}
			for _, ref := range refs {
				if ref.Kind != placeholder.KindResource {
					continue
				}
				target, err := resolve(ref, id, path)
				if err != nil {
					return err
				}
				b.addEdge(workload.Descriptor, target.Descriptor, ReasonWorkloadPlace, path)
				b.bind(workload, ref.Resource, target.Descriptor)
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Resource params may read a sibling resource's outputs.
		for _, name := range module.ExternalNames() {
			consumer := privateNodes[id][name]
			base := pointer("modules", id, "externals", name, "params")
			if err := b.scanParams(consumer, module.Externals[name].Params, base, id, resolve); err != nil {
				return err
			}
		}
	}

	for _, id := range set.SharedIDs() {
		consumer := sharedNodes[id]
		base := pointer("shared", id, "params")
		if err := b.scanParams(consumer, set.Shared[id].Params, base, "", resolve); err != nil {
			return err
		}
	}
	return nil
}

func (b *graphBuilder) scanParams(
	consumer *Node,
	params map[string]any,
	base, moduleID string,
	resolve func(ref placeholder.Ref, moduleID, path string) (*Node, error),
) error {
	if len(params) == 0 {
		return nil
	}
	tree, err := canon.Map(params)
	if err != nil {
		return err
	}
	return placeholder.WalkStrings(tree, base, func(path, value string) error {
		refs, err := placeholder.Refs(value)
		if err != nil {
			return fmt.Errorf("planning: %s: %w", path, err)
		}
		for _, ref := range refs {
			if ref.Kind != placeholder.KindResource {
				continue
			}
			target, err := resolve(ref, moduleID, path)
			if err != nil {
				return err
			}
			b.addEdge(consumer.Descriptor, target.Descriptor, ReasonInputPlace, path)
			b.bind(consumer, ref.Resource, target.Descriptor)
		}
		return nil
	})
}

func copyParams(params map[string]any) map[string]any {
	if len(params) == 0 {
		return map[string]any{}
	}
	out, err := canon.Map(params)
	if err != nil {
		return map[string]any{}
	}
	return out
}

func descriptorID(descriptor string) string {
	d, err := resource.ParseDescriptor(descriptor)
	if err != nil {
		return ""
	}
	return d.ID
}

func pointer(segments ...string) string {
	var b strings.Builder
	for _, segment := range segments {
		b.WriteString("/")
		b.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(segment))
	}
	return b.String()
}
