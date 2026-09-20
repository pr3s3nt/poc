package planner

import (
	"strings"
)

type descriptor struct {
	Type       string
	Class      string
	ResourceID string
}

func (d descriptor) String() string { return d.Type + "." + d.Class + "#" + d.ResourceID }

// parseDescriptor reads TYPE[.CLASS][#ID]. A missing or "@" class or ID is
// inherited from the current node.
func parseDescriptor(raw string, current *Node, path string) (descriptor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return descriptor{}, newError("graph", "INVALID_RESOURCE_REFERENCE", path)
	}
	if strings.ContainsAny(raw, "<>") {
		return descriptor{}, newError("graph", "UNSUPPORTED_SELECTOR", path)
	}

	head, id := raw, ""
	if idx := strings.Index(raw, "#"); idx >= 0 {
		head, id = raw[:idx], raw[idx+1:]
	}
	typ, class := head, ""
	if idx := strings.Index(head, "."); idx >= 0 {
		typ, class = head[:idx], head[idx+1:]
	}
	if typ == "" {
		return descriptor{}, newError("graph", "INVALID_RESOURCE_REFERENCE", path)
	}
	if class == "" || class == "@" {
		class = "default"
		if current != nil {
			class = current.Class
		}
	}
	if id == "" || id == "@" {
		if current == nil {
			return descriptor{}, newError("graph", "INVALID_RESOURCE_REFERENCE", path)
		}
		id = current.ResourceID
	}
	return descriptor{Type: typ, Class: class, ResourceID: id}, nil
}

type resourceReference struct {
	Raw    string // descriptor text as written
	Output string
}

// parseResourceReference reads the body of a ${...} placeholder written as
// ${resources['TYPE[.CLASS][#ID]'].outputs.OUTPUT} or
// ${resources.TYPE.outputs.OUTPUT}.
func parseResourceReference(body string) (resourceReference, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(body), "resources")
	if !ok {
		return resourceReference{}, false
	}
	var raw string
	switch {
	case strings.HasPrefix(rest, "['"), strings.HasPrefix(rest, "[\""):
		quote := rest[1]
		end := strings.Index(rest[2:], string(quote)+"]")
		if end < 0 {
			return resourceReference{}, false
		}
		raw = rest[2 : 2+end]
		rest = rest[2+end+2:]
	case strings.HasPrefix(rest, "."):
		rest = rest[1:]
		idx := strings.Index(rest, ".")
		if idx < 0 {
			return resourceReference{}, false
		}
		raw = rest[:idx]
		rest = rest[idx:]
	default:
		return resourceReference{}, false
	}
	outputs, ok := strings.CutPrefix(rest, ".outputs.")
	if !ok || outputs == "" {
		return resourceReference{}, false
	}
	return resourceReference{Raw: raw, Output: outputs}, true
}

// resolveContext replaces ${context.*} placeholders. Resource references and
// any other placeholder are left untouched.
func resolveContext(s string, ctx Document, node *Node) string {
	var b strings.Builder
	cursor := 0
	for _, ph := range scanPlaceholders(s) {
		value, ok := contextValue(ph.Body, ctx, node)
		if !ok {
			continue
		}
		b.WriteString(s[cursor:ph.Start])
		b.WriteString(value)
		cursor = ph.End
	}
	b.WriteString(s[cursor:])
	return b.String()
}

func contextValue(body string, ctx Document, node *Node) (string, bool) {
	switch strings.TrimSpace(body) {
	case "context.org.id":
		return stringAt(ctx, "org_id"), true
	case "context.app.id":
		return stringAt(ctx, "app_id"), true
	case "context.env.id":
		return stringAt(ctx, "env_id"), true
	case "context.env.type":
		return stringAt(ctx, "env_type"), true
	case "context.res.id":
		return node.ResourceID, true
	case "context.res.class":
		return node.Class, true
	case "context.res.type":
		return node.Type, true
	}
	return "", false
}

// setPlaceholder is a ${externals.<name>[.<output>]} or
// ${shared.<id>[.<output>]} reference inside a Deployment Set.
type setPlaceholder struct {
	Scope  string
	Name   string
	Output string
}

// Outcomes of parsing a Deployment Set placeholder.
const (
	setPlaceholderNone      = iota // not a Deployment Set placeholder at all
	setPlaceholderValid            // resources.<name> shape, well formed
	setPlaceholderBadName          // scope without a resource name
	setPlaceholderBadOutput        // more segments than <scope>.<name>.<output>
)

// parseSetPlaceholder accepts exactly "<scope>.<name>" and
// "<scope>.<name>.<output>"; anything longer or shorter is rejected so a typo
// cannot silently drop a graph edge.
func parseSetPlaceholder(body string) (setPlaceholder, int) {
	segments := strings.Split(strings.TrimSpace(body), ".")
	if segments[0] != "externals" && segments[0] != "shared" {
		return setPlaceholder{}, setPlaceholderNone
	}
	switch {
	case len(segments) < 2 || segments[1] == "":
		return setPlaceholder{}, setPlaceholderBadName
	case len(segments) > 3:
		return setPlaceholder{}, setPlaceholderBadOutput
	}
	ph := setPlaceholder{Scope: segments[0], Name: segments[1]}
	if len(segments) == 3 {
		if segments[2] == "" {
			return setPlaceholder{}, setPlaceholderBadOutput
		}
		ph.Output = segments[2]
	}
	return ph, setPlaceholderValid
}
