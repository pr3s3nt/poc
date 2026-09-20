package planner

import "sort"

func newGraph() *ResourceGraph {
	return &ResourceGraph{Nodes: map[string]*Node{}, seen: map[string]bool{}}
}

func (g *ResourceGraph) ensure(d descriptor, origin string, inputs Document) *Node {
	key := d.String()
	node, ok := g.Nodes[key]
	if !ok {
		if inputs == nil {
			inputs = Document{}
		}
		node = &Node{Type: d.Type, Class: d.Class, ResourceID: d.ResourceID, ResourceInputs: inputs}
		g.Nodes[key] = node
	}
	node.addOrigin(origin)
	return node
}

func (n *Node) addOrigin(origin string) {
	if origin == "" {
		return
	}
	for _, existing := range n.Origins {
		if existing == origin {
			return
		}
	}
	n.Origins = append(n.Origins, origin)
	sort.Strings(n.Origins)
}

// addEdge records a consumer -> provider edge, ignoring exact duplicates.
func (g *ResourceGraph) addEdge(from, to, reason, path string) bool {
	key := from + "\x00" + to + "\x00" + reason + "\x00" + path
	if g.seen[key] {
		return false
	}
	g.seen[key] = true
	g.Edges = append(g.Edges, Edge{From: from, To: to, Reason: reason, Path: path})
	return true
}

func (g *ResourceGraph) descriptors() []string {
	out := make([]string, 0, len(g.Nodes))
	for key := range g.Nodes {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// consumersOf returns every node that already depends on the given provider.
func (g *ResourceGraph) consumersOf(provider string) []string {
	set := map[string]bool{}
	for _, e := range g.Edges {
		if e.To == provider {
			set[e.From] = true
		}
	}
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func resourceEntryDescriptor(entry Document, resID string) descriptor {
	class := stringAt(entry, "class")
	if class == "" {
		class = "default"
	}
	return descriptor{Type: stringAt(entry, "type"), Class: class, ResourceID: resID}
}

func entryParams(entry Document) Document {
	params := mapAt(entry, "params")
	if params == nil {
		return Document{}
	}
	return asDocument(deepCopy(params))
}

// buildInitialGraph derives nodes and edges from the Candidate Deployment Set.
func buildInitialGraph(candidate Document, types map[string]*ResourceType) (*ResourceGraph, error) {
	g := newGraph()
	modules := mapAt(candidate, "modules")
	shared := mapAt(candidate, "shared")

	// Nodes first, so placeholder scanning can resolve every target.
	moduleNodes := map[string]*Node{}
	privateNodes := map[string]map[string]*Node{}
	for _, id := range sortedKeys(modules) {
		module := asDocument(modules[id])
		workload := g.ensure(descriptor{Type: "workload", Class: "default", ResourceID: "modules." + id},
			"implicit-workload", Document{})
		moduleNodes[id] = workload
		privateNodes[id] = map[string]*Node{}
		externals := mapAt(module, "externals")
		for _, name := range sortedKeys(externals) {
			entry := asDocument(externals[name])
			d := resourceEntryDescriptor(entry, "modules."+id+".externals."+name)
			privateNodes[id][name] = g.ensure(d, "private-dependency", entryParams(entry))
		}
	}
	sharedNodes := map[string]*Node{}
	for _, name := range sortedKeys(shared) {
		entry := asDocument(shared[name])
		d := resourceEntryDescriptor(entry, "shared."+name)
		sharedNodes[name] = g.ensure(d, "shared-dependency", entryParams(entry))
	}

	// resolve maps a Deployment Set placeholder onto its provider node and
	// checks the referenced output against the Resource Type schema.
	resolve := func(ph setPlaceholder, kind int, moduleID, path string) (*Node, error) {
		switch kind {
		case setPlaceholderBadName:
			return nil, newError("graph", "UNKNOWN_RESOURCE", path)
		case setPlaceholderBadOutput:
			return nil, newError("graph", "UNKNOWN_OUTPUT", path)
		}
		var node *Node
		if ph.Scope == "shared" {
			node = sharedNodes[ph.Name]
		} else if moduleID != "" {
			node = privateNodes[moduleID][ph.Name]
		}
		if node == nil {
			return nil, newError("graph", "UNKNOWN_RESOURCE", path)
		}
		if ph.Output != "" {
			if rt, known := types[node.Type]; known {
				if _, ok := rt.Outputs[ph.Output]; !ok {
					return nil, newError("graph", "UNKNOWN_OUTPUT", path)
				}
			}
		}
		return node, nil
	}

	for _, id := range sortedKeys(modules) {
		module := asDocument(modules[id])
		workload := moduleNodes[id]

		// Private dependency edges follow the Deployment Set structure.
		externals := mapAt(module, "externals")
		for _, name := range sortedKeys(externals) {
			target := privateNodes[id][name]
			g.addEdge(workload.Descriptor(), target.Descriptor(), "deployment-set-private",
				pointer("modules", id, "externals", name))
		}

		// Workload placeholders read resource outputs from the module spec.
		err := walkStrings(module["spec"], pointer("modules", id, "spec"), func(path, value string) error {
			for _, ph := range scanPlaceholders(value) {
				parsed, kind := parseSetPlaceholder(ph.Body)
				if kind == setPlaceholderNone {
					continue
				}
				target, err := resolve(parsed, kind, id, path)
				if err != nil {
					return err
				}
				g.addEdge(workload.Descriptor(), target.Descriptor(), "workload-placeholder", path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		// Resource inputs may read a sibling resource's outputs.
		for _, name := range sortedKeys(externals) {
			entry := asDocument(externals[name])
			consumer := privateNodes[id][name]
			base := pointer("modules", id, "externals", name, "params")
			if err := scanInputPlaceholders(g, entry["params"], base, consumer, id, resolve); err != nil {
				return nil, err
			}
		}
	}

	for _, name := range sortedKeys(shared) {
		entry := asDocument(shared[name])
		consumer := sharedNodes[name]
		base := pointer("shared", name, "params")
		if err := scanInputPlaceholders(g, entry["params"], base, consumer, "", resolve); err != nil {
			return nil, err
		}
	}

	return g, nil
}

func scanInputPlaceholders(g *ResourceGraph, params any, base string, consumer *Node, moduleID string,
	resolve func(ph setPlaceholder, kind int, moduleID, path string) (*Node, error)) error {
	if params == nil {
		return nil
	}
	return walkStrings(params, base, func(path, value string) error {
		for _, ph := range scanPlaceholders(value) {
			parsed, kind := parseSetPlaceholder(ph.Body)
			if kind == setPlaceholderNone {
				continue
			}
			target, err := resolve(parsed, kind, moduleID, path)
			if err != nil {
				return err
			}
			g.addEdge(consumer.Descriptor(), target.Descriptor(), "resource-input-placeholder", path)
		}
		return nil
	})
}

// topologicalBatches returns Kahn batches; providers always land in an earlier
// batch than their consumers.
func topologicalBatches(g *ResourceGraph) ([][]string, error) {
	pending := map[string]map[string]bool{}
	for key := range g.Nodes {
		pending[key] = map[string]bool{}
	}
	for _, e := range g.Edges {
		if e.From == e.To {
			continue
		}
		if _, ok := pending[e.From]; ok {
			if _, ok := g.Nodes[e.To]; ok {
				pending[e.From][e.To] = true
			}
		}
	}

	batches := [][]string{}
	for len(pending) > 0 {
		var batch []string
		for key, deps := range pending {
			if len(deps) == 0 {
				batch = append(batch, key)
			}
		}
		if len(batch) == 0 {
			return nil, newError("schedule", "RESOURCE_GRAPH_CYCLE", "")
		}
		sort.Strings(batch)
		for _, key := range batch {
			delete(pending, key)
		}
		for _, deps := range pending {
			for _, key := range batch {
				delete(deps, key)
			}
		}
		batches = append(batches, batch)
	}
	return batches, nil
}
