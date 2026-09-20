package planner

import "sort"

// RenderResult assembles the single JSON object written to stdout.
func RenderResult(delta, candidate Document, g *ResourceGraph, records []*TerraformRecord,
	classification Document, batches [][]string) Result {

	matched := Document{}
	nodes := []any{}
	for _, key := range g.descriptors() {
		node := g.Nodes[key]
		origins := []any{}
		for _, origin := range node.Origins {
			origins = append(origins, origin)
		}
		inputs := node.ResourceInputs
		if inputs == nil {
			inputs = Document{}
		}
		nodes = append(nodes, Document{
			"descriptor":     key,
			"origins":        origins,
			"resourceInputs": inputs,
		})
		if node.Definition == nil {
			continue
		}
		criterion := node.Criterion
		if criterion == nil {
			criterion = Document{}
		}
		matched[key] = Document{
			"definitionId": node.Definition.ID,
			"score":        node.Score,
			"criterion":    criterion,
		}
	}

	edges := append([]Edge(nil), g.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		if a.Reason != b.Reason {
			return a.Reason < b.Reason
		}
		return a.Path < b.Path
	})
	edgeList := []any{}
	for _, e := range edges {
		doc := Document{"from": e.From, "to": e.To, "reason": e.Reason}
		if e.Path != "" {
			doc["path"] = e.Path
		}
		edgeList = append(edgeList, doc)
	}

	sort.Slice(records, func(i, j int) bool { return records[i].Resource < records[j].Resource })
	terraform := []any{}
	for _, record := range records {
		outputs := []any{}
		for _, name := range record.Outputs {
			outputs = append(outputs, name)
		}
		inputs := []any{}
		for _, input := range record.Inputs {
			inputs = append(inputs, input)
		}
		terraform = append(terraform, Document{
			"resource":       record.Resource,
			"definitionId":   record.DefinitionID,
			"source":         record.Source,
			"localDirectory": record.Directory,
			"fingerprint":    record.Fingerprint,
			"inputs":         inputs,
			"outputs":        outputs,
		})
	}

	batchList := []any{}
	for _, batch := range batches {
		items := []any{}
		for _, descriptor := range batch {
			items = append(items, descriptor)
		}
		batchList = append(batchList, items)
	}

	plan := Document{
		"matchedDefinitions": matched,
		"resourceGraph":      Document{"nodes": nodes, "edges": edgeList},
		"terraform":          terraform,
		"provisionBatches":   batchList,
		"activeResources":    classification,
	}

	return Result{
		"status":        "ACCEPTED",
		"delta":         delta,
		"deploymentSet": candidate,
		"challengePlan": plan,
	}
}
