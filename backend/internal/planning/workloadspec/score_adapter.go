package workloadspec

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

var ErrScoreSchema = errors.New("workload: invalid Score schema")
var ErrScoreUnsupported = errors.New("workload: unsupported Score feature")

const ScoreAPIVersion = "score.dev/v1b1"
const ExtensionAPIVersion = "humanitec.org/v1b1"

//go:embed schema/score-v1b1.json
var scoreSchemaJSON string

var scoreSchema = func() *jsonschema.Schema {
	c := jsonschema.NewCompiler()
	c.LoadURL = func(string) (io.ReadCloser, error) { return nil, fmt.Errorf("external schemas are disabled") }
	if err := c.AddResource("score.json", strings.NewReader(scoreSchemaJSON)); err != nil {
		panic(err)
	}
	s, err := c.Compile("score.json")
	if err != nil {
		panic(err)
	}
	return s
}()

// ScoreDocument normalizes portable intent, without choosing infrastructure or
// reading local files. The caller resolves Profile against its scoped source.
type ScoreDocument struct {
	Workload map[string]any
	Profile  string
}

func jsonCopy(in any, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(out)
}
func scoreInvalid(format string, args ...any) error {
	return fmt.Errorf("%w: %w: %s", ErrInvalid, ErrScoreUnsupported, fmt.Sprintf(format, args...))
}
func mapObject(v any) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return nil, scoreInvalid("expected object")
	}
	return m, nil
}
func allowed(m map[string]any, keys ...string) error {
	for k := range m {
		found := false
		for _, key := range keys {
			found = found || k == key
		}
		if !found {
			return scoreInvalid("unsupported field %q", k)
		}
	}
	return nil
}

// DecodeScore validates the actual upstream schema first. Unsupported valid
// features are errors; they are never silently interpreted as native fields.
func DecodeScore(input, extension map[string]any) (ScoreDocument, error) {
	var m map[string]any
	if err := jsonCopy(input, &m); err != nil {
		return ScoreDocument{}, scoreInvalid("invalid Score")
	}
	if err := scoreSchema.Validate(m); err != nil {
		return ScoreDocument{}, fmt.Errorf("%w: %w: Score schema: %v", ErrInvalid, ErrScoreSchema, err)
	}
	meta, _ := mapObject(m["metadata"])
	if err := allowed(meta, "name", "annotations"); err != nil {
		return ScoreDocument{}, err
	}
	// Expand the supported metadata reference before the existing resource
	// placeholder parser. Escaped references remain literal.
	expand := func(value string) string {
		const marker = "${metadata.name}"
		var out strings.Builder
		for i := 0; i < len(value); {
			if strings.HasPrefix(value[i:], "$${") {
				end := strings.IndexByte(value[i+3:], '}')
				if end >= 0 {
					end += i + 4
					out.WriteString(value[i:end])
					i = end
					continue
				}
			}
			if strings.HasPrefix(value[i:], marker) {
				out.WriteString(meta["name"].(string))
				i += len(marker)
			} else {
				out.WriteByte(value[i])
				i++
			}
		}
		return out.String()
	}
	if cs, ok := m["containers"].(map[string]any); ok {
		for _, v := range cs {
			c := v.(map[string]any)
			if vars, ok := c["variables"].(map[string]any); ok {
				for k, v := range vars {
					vars[k] = expand(v.(string))
				}
			}
		}
	}
	var expandTree func(any) any
	expandTree = func(v any) any {
		switch x := v.(type) {
		case string:
			return expand(x)
		case map[string]any:
			for k, v := range x {
				x[k] = expandTree(v)
			}
		case []any:
			for k, v := range x {
				x[k] = expandTree(v)
			}
		}
		return v
	}
	if rs, ok := m["resources"].(map[string]any); ok {
		for _, v := range rs {
			r := v.(map[string]any)
			if params, ok := r["params"]; ok {
				r["params"] = expandTree(params)
			}
		}
	}
	m["apiVersion"] = SupportedAPIVersion
	containers, _ := mapObject(m["containers"])
	for _, v := range containers {
		c, _ := mapObject(v)
		if _, ok := c["files"]; ok {
			return ScoreDocument{}, scoreInvalid("Score files are not supported; use explicit native file mounts")
		}
		if _, ok := c["volumes"]; ok {
			return ScoreDocument{}, scoreInvalid("Score volumes are not supported; use explicit native volume mounts")
		}
		for _, key := range []string{"readinessProbe", "livenessProbe"} {
			if p, ok := c[key]; ok {
				n, err := importProbe(p)
				if err != nil {
					return ScoreDocument{}, err
				}
				c[key] = n
			}
		}
	}
	if resources, ok := m["resources"].(map[string]any); ok {
		for _, v := range resources {
			r, _ := mapObject(v)
			if _, ok := r["metadata"]; ok {
				return ScoreDocument{}, scoreInvalid("resource metadata is not supported")
			}
		}
	}
	profile := "humanitec/default-module"
	if extension != nil {
		if err := allowed(extension, "apiVersion", "profile", "spec"); err != nil {
			return ScoreDocument{}, err
		}
		if extension["apiVersion"] != ExtensionAPIVersion {
			return ScoreDocument{}, scoreInvalid("unsupported extension apiVersion")
		}
		if p, ok := extension["profile"]; ok {
			var good bool
			profile, good = p.(string)
			if !good || profile == "" {
				return ScoreDocument{}, scoreInvalid("profile must be non-empty")
			}
		}
		if spec, ok := extension["spec"]; ok {
			s, err := mapObject(spec)
			if err != nil {
				return ScoreDocument{}, err
			}
			if err := applyExtension(m, s); err != nil {
				return ScoreDocument{}, err
			}
		}
	}
	// A profile selects authoring metadata, never class or resource declarations.
	if profile == "humanitec/default-cronjob" {
		e := execution(m)
		if k, ok := e["kind"]; ok && k != "CronJob" {
			return ScoreDocument{}, scoreInvalid("profile conflicts with runtime kind")
		}
		e["kind"] = "CronJob"
	}
	if strings.HasPrefix(profile, "humanitec/") && profile != "humanitec/default-module" && profile != "humanitec/default-cronjob" {
		return ScoreDocument{}, scoreInvalid("unsupported Humanitec profile")
	}
	return ScoreDocument{Workload: m, Profile: profile}, nil
}

func importProbe(v any) (map[string]any, error) {
	p, err := mapObject(v)
	if err != nil {
		return nil, err
	}
	if err := allowed(p, "httpGet", "exec"); err != nil {
		return nil, err
	}
	if len(p) != 1 {
		return nil, scoreInvalid("probe requires one action")
	}
	if h, ok := p["httpGet"]; ok {
		a, err := mapObject(h)
		if err != nil {
			return nil, err
		}
		if err := allowed(a, "path", "port", "scheme"); err != nil {
			return nil, err
		}
		out := map[string]any{"path": a["path"], "port": a["port"]}
		if s, ok := a["scheme"]; ok {
			out["scheme"] = s
		}
		return out, nil
	}
	a, err := mapObject(p["exec"])
	if err != nil {
		return nil, err
	}
	return map[string]any{"type": "exec", "command": a["command"]}, nil
}
func execution(m map[string]any) map[string]any {
	e, ok := m["workloadExecution"].(map[string]any)
	if !ok {
		e = map[string]any{"version": 1}
		m["workloadExecution"] = e
	}
	return e
}
func put(m map[string]any, k string, v any) error {
	if _, exists := m[k]; exists {
		return scoreInvalid("overlapping extension field %q", k)
	}
	m[k] = v
	return nil
}
func moveFields(dst, src map[string]any, fields map[string]string) error {
	for k, v := range src {
		name, ok := fields[k]
		if !ok {
			return scoreInvalid("unsupported extension field %q", k)
		}
		if err := put(dst, name, v); err != nil {
			return err
		}
	}
	return nil
}
func applyExtension(m, s map[string]any) error {
	if err := allowed(s, "deployment", "pod", "containers", "schedules", "cronjob", "orchestrator"); err != nil {
		return err
	}
	if native, ok := s["orchestrator"]; ok {
		n, err := mapObject(native)
		if err != nil {
			return err
		}
		if err := allowed(n, "metadata", "replicas", "containers", "workloadExecution"); err != nil {
			return err
		}
		for k, v := range n {
			switch k {
			case "metadata":
				md, err := mapObject(v)
				if err != nil {
					return err
				}
				if err := allowed(md, "class"); err != nil {
					return err
				}
				for key, value := range md {
					m["metadata"].(map[string]any)[key] = value
				}
			case "containers":
				cs, err := mapObject(v)
				if err != nil {
					return err
				}
				for name, fields := range cs {
					target, ok := m["containers"].(map[string]any)[name].(map[string]any)
					if !ok {
						return scoreInvalid("unknown container")
					}
					f, err := mapObject(fields)
					if err != nil {
						return err
					}
					if err := allowed(f, "role", "initOrder", "workingDir", "ports", "securityContext", "startupProbe", "readinessProbe", "livenessProbe", "variableOrder"); err != nil {
						return err
					}
					for key, value := range f {
						if err := put(target, key, value); err != nil {
							return err
						}
					}
				}
			default:
				if k == "workloadExecution" {
					e, err := mapObject(v)
					if err != nil {
						return err
					}
					if _, ok := e["templateDefaults"]; ok {
						return scoreInvalid("templateDefaults is platform-owned")
					}
				}
				m[k] = v
			}
		}
	}
	if v, ok := s["deployment"]; ok {
		d, err := mapObject(v)
		if err != nil {
			return err
		}
		for k, value := range d {
			if k == "replicas" {
				if err := put(m, k, value); err != nil {
					return err
				}
			} else {
				if err := moveFields(execution(m), map[string]any{k: value}, map[string]string{"strategy": "updateStrategy", "revisionHistoryLimit": "revisionHistoryLimit"}); err != nil {
					return err
				}
			}
		}
		e := execution(m)
		if kind, ok := e["kind"]; ok && kind != "Deployment" {
			return scoreInvalid("deployment conflicts with runtime kind")
		}
		e["kind"] = "Deployment"
	}
	if v, ok := s["pod"]; ok {
		p, err := mapObject(v)
		if err != nil {
			return err
		}
		if err := moveFields(execution(m), p, map[string]string{"securityContext": "podSecurityContext", "automountServiceAccountToken": "automountServiceAccountToken", "serviceAccountName": "serviceAccountName"}); err != nil {
			return err
		}
	}
	if v, ok := s["containers"]; ok {
		cs, err := mapObject(v)
		if err != nil {
			return err
		}
		for name, value := range cs {
			target, ok := m["containers"].(map[string]any)[name].(map[string]any)
			if !ok {
				return scoreInvalid("unknown container")
			}
			p, err := mapObject(value)
			if err != nil {
				return err
			}
			if err := moveFields(target, p, map[string]string{"securityContext": "securityContext"}); err != nil {
				return err
			}
		}
	}
	if v, ok := s["schedules"]; ok {
		schedules, err := mapObject(v)
		if err != nil {
			return err
		}
		if len(schedules) != 1 {
			return scoreInvalid("exactly one schedule is supported per workload")
		}
		for _, value := range schedules {
			sched, err := mapObject(value)
			if err != nil {
				return err
			}
			if err := allowed(sched, "schedule", "containers"); err != nil {
				return err
			}
			e := execution(m)
			if k, ok := e["kind"]; ok && k != "CronJob" {
				return scoreInvalid("schedule conflicts with runtime kind")
			}
			e["kind"] = "CronJob"
			if err := put(e, "schedule", map[string]any{"expression": sched["schedule"], "suspend": false}); err != nil {
				return err
			}
			if v, ok := sched["containers"]; ok {
				overrides, err := mapObject(v)
				if err != nil {
					return err
				}
				for name, value := range overrides {
					target, ok := m["containers"].(map[string]any)[name].(map[string]any)
					if !ok {
						return scoreInvalid("unknown schedule container")
					}
					p, err := mapObject(value)
					if err != nil {
						return err
					}
					if err := allowed(p, "command", "args"); err != nil {
						return err
					}
					for k, v := range p {
						target[k] = v
					}
				}
			}
		}
	}
	if v, ok := s["cronjob"]; ok {
		p, err := mapObject(v)
		if err != nil {
			return err
		}
		e := execution(m)
		if e["kind"] != "CronJob" {
			return scoreInvalid("cronjob requires a schedule")
		}
		sched, err := mapObject(e["schedule"])
		if err != nil {
			return err
		}
		for k, v := range p {
			switch k {
			case "timeZone", "concurrencyPolicy", "startingDeadlineSeconds", "successfulJobsHistoryLimit", "failedJobsHistoryLimit":
				if err := put(sched, k, v); err != nil {
					return err
				}
			case "suspend":
				// The false inserted by schedules is a normalization default;
				// an explicitly authored native value must not be overwritten.
				if native, ok := s["orchestrator"].(map[string]any); ok {
					if execution, ok := native["workloadExecution"].(map[string]any); ok {
						if schedule, ok := execution["schedule"].(map[string]any); ok {
							if _, present := schedule["suspend"]; present {
								return scoreInvalid("overlapping extension field suspend")
							}
						}
					}
				}
				sched[k] = v
			default:
				return scoreInvalid("unsupported CronJob field %q", k)
			}
		}
	}
	if e, ok := m["workloadExecution"].(map[string]any); ok {
		if _, ok := e["kind"]; !ok {
			e["kind"] = "Deployment"
		}
	}
	return nil
}

// ExportScore preserves every supported native field in an explicit extension.
// Round-trip validation makes loss of intent an error, including legacy shapes
// which cannot be represented by the upstream Score schema.
func ExportScore(doc *Document) (map[string]any, map[string]any, error) {
	original := doc.Map()
	var score map[string]any
	if err := jsonCopy(original, &score); err != nil {
		return nil, nil, err
	}
	score["apiVersion"] = ScoreAPIVersion
	native := map[string]any{}
	for _, k := range []string{"replicas", "workloadExecution"} {
		if v, ok := score[k]; ok {
			native[k] = v
			delete(score, k)
		}
	}
	if e, ok := native["workloadExecution"].(map[string]any); ok {
		delete(e, "templateDefaults")
	}
	meta := score["metadata"].(map[string]any)
	if v, ok := meta["class"]; ok {
		native["metadata"] = map[string]any{"class": v}
		delete(meta, "class")
	}
	extra := map[string]any{}
	for name, v := range score["containers"].(map[string]any) {
		c := v.(map[string]any)
		fields := map[string]any{}
		for k, v := range c {
			switch k {
			case "image", "command", "args", "variables", "resources":
			default:
				fields[k] = v
				delete(c, k)
			}
		}
		if len(fields) > 0 {
			extra[name] = fields
		}
	}
	if len(extra) > 0 {
		native["containers"] = extra
	}
	extension := map[string]any{"apiVersion": ExtensionAPIVersion, "profile": "orchestrator/custom", "spec": map[string]any{"orchestrator": native}}
	if e := doc.Execution; e.HasTemplate() {
		extension["profile"] = e.Template.ID + "@" + e.Template.Version
	}
	converted, err := DecodeScore(score, extension)
	if err != nil {
		return nil, nil, err
	}
	restored, err := FromMap(converted.Workload)
	if err != nil {
		return nil, nil, err
	}
	expected := doc.Map()
	if e := doc.Execution; e != nil && e.TemplateDefaults != nil {
		var cp map[string]any
		_ = jsonCopy(expected, &cp)
		delete(cp["workloadExecution"].(map[string]any), "templateDefaults")
		expected = cp
	}
	if !reflect.DeepEqual(canonicalJSON(restored.Map()), canonicalJSON(expected)) {
		return nil, nil, scoreInvalid("workload cannot be exported losslessly")
	}
	return score, extension, nil
}
func canonicalJSON(v any) any { var out any; _ = jsonCopy(v, &out); return out }
