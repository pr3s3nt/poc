// Package drift parses private Terraform plans into value-free findings. It is
// separate from the cleanup parser, whose disappeared-object contract remains.
package drift

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

var ErrPlan = errors.New("drift: incomplete or unsupported private plan")

type Finding struct {
	Address string   `json:"address"`
	Type    string   `json:"type"`
	Actions []string `json:"actions"`
	Paths   []string `json:"paths"`
}
type Report struct {
	External       []Finding `json:"externalDrift"`
	Proposed       []Finding `json:"proposedActions"`
	OutputNames    []string  `json:"outputNames"`
	Complete       bool      `json:"complete"`
	OutputsUnknown bool      `json:"outputsUnknown"`
	Applyable      bool      `json:"-"`
	// Only State runtime may persist/compare this secret-dependent digest. Never
	// serialize it with public findings or send it to Console.
	PrivateChangeDigest string `json:"-"`
}
type change struct {
	Actions         []string `json:"actions"`
	Before          any      `json:"before"`
	After           any      `json:"after"`
	AfterUnknown    any      `json:"after_unknown"`
	BeforeSensitive any      `json:"before_sensitive"`
	AfterSensitive  any      `json:"after_sensitive"`
	Importing       any      `json:"importing"`
}
type resourceChange struct {
	Address         string `json:"address"`
	Mode            string `json:"mode"`
	Type            string `json:"type"`
	PreviousAddress string `json:"previous_address"`
	Deposed         string `json:"deposed"`
	Change          change `json:"change"`
}
type document struct {
	Format    string            `json:"format_version"`
	Complete  *bool             `json:"complete"`
	Errored   bool              `json:"errored"`
	Applyable *bool             `json:"applyable"`
	Deferred  []json.RawMessage `json:"deferred_changes"`
	Resources []resourceChange  `json:"resource_changes"`
	Drift     []resourceChange  `json:"resource_drift"`
	Outputs   map[string]change `json:"output_changes"`
}

func active(actions []string) bool {
	for _, a := range actions {
		if a != "no-op" && a != "read" {
			return true
		}
	}
	return false
}
func unknown(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case map[string]any:
		for _, v := range x {
			if unknown(v) {
				return true
			}
		}
	case []any:
		for _, v := range x {
			if unknown(v) {
				return true
			}
		}
	}
	return false
}
func pointer(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }
func paths(before, after, beforeSensitive, afterSensitive any, prefix string, out *[]string) {
	if len(*out) >= 200 {
		return
	}
	if reflect.DeepEqual(before, after) {
		return
	}
	if beforeSensitive == true || afterSensitive == true {
		if prefix == "" {
			prefix = "/"
		}
		*out = append(*out, prefix)
		return
	}
	a, aok := before.(map[string]any)
	b, bok := after.(map[string]any)
	if aok && bok {
		keys := map[string]bool{}
		for k := range a {
			keys[k] = true
		}
		for k := range b {
			keys[k] = true
		}
		sorted := []string{}
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			path := prefix + "/" + pointer(k)
			if len(path) > 256 {
				path = prefix + "/<key>"
			}
			paths(a[k], b[k], sensitivityField(beforeSensitive, k), sensitivityField(afterSensitive, k), path, out)
		}
		return
	}
	aa, aok := before.([]any)
	bb, bok := after.([]any)
	if aok && bok && len(aa) == len(bb) {
		for i := range aa {
			paths(aa[i], bb[i], sensitivityIndex(beforeSensitive, i), sensitivityIndex(afterSensitive, i), prefix+"/"+strconv.Itoa(i), out)
		}
		return
	}
	if prefix == "" {
		prefix = "/"
	}
	*out = append(*out, prefix)
}
func sensitivityField(mask any, key string) any {
	if m, ok := mask.(map[string]any); ok {
		return m[key]
	}
	return nil
}
func sensitivityIndex(mask any, index int) any {
	if m, ok := mask.([]any); ok && index < len(m) {
		return m[index]
	}
	return nil
}
func findings(changes []resourceChange) ([]Finding, error) {
	result := []Finding{}
	for _, r := range changes {
		if r.Mode != "" && r.Mode != "managed" {
			continue
		}
		if r.Address == "" || r.Type == "" || len(r.Address) > 1024 || len(r.Type) > 256 || len(r.Change.Actions) == 0 {
			return nil, ErrPlan
		}
		for _, a := range r.Change.Actions {
			switch a {
			case "no-op", "read", "create", "update", "delete", "forget":
			default:
				return nil, ErrPlan
			}
		}
		if !active(r.Change.Actions) && r.Change.Importing == nil && r.PreviousAddress == "" && r.Deposed == "" {
			continue
		}
		fields := []string{}
		paths(r.Change.Before, r.Change.After, r.Change.BeforeSensitive, r.Change.AfterSensitive, "", &fields)
		if len(fields) >= 200 {
			return nil, ErrPlan
		}
		for _, field := range fields {
			if strings.Contains(field, "/<key>") {
				return nil, ErrPlan
			}
		}
		if unknown(r.Change.AfterUnknown) {
			fields = append(fields, "/<unknown>")
		}
		if r.Change.Importing != nil {
			fields = append(fields, "/<import>")
		}
		if r.PreviousAddress != "" || r.Deposed != "" {
			fields = append(fields, "/<identity>")
		}
		result = append(result, Finding{r.Address, r.Type, append([]string(nil), r.Change.Actions...), fields})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Address < result[j].Address })
	return result, nil
}

// Parse never returns a partial report as clean. The private digest covers all
// semantic change values, unknown/sensitive markers and imports; timestamps,
// Terraform version, UI hints and resource ordering cannot change it.
func Parse(raw []byte) (Report, error) {
	if len(raw) > 32<<20 {
		return Report{}, ErrPlan
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var doc document
	if decoder.Decode(&doc) != nil {
		return Report{}, ErrPlan
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return Report{}, ErrPlan
	}
	format := strings.Split(doc.Format, ".")
	validFormat := len(format) == 2 && format[0] == "1"
	if validFormat {
		_, err := strconv.ParseUint(format[1], 10, 32)
		validFormat = err == nil
	}
	if !validFormat || doc.Errored || (doc.Complete != nil && !*doc.Complete) || len(doc.Deferred) > 0 {
		return Report{}, ErrPlan
	}
	external, err := findings(doc.Drift)
	if err != nil {
		return Report{}, err
	}
	proposed, err := findings(doc.Resources)
	if err != nil {
		return Report{}, err
	}
	report := Report{External: external, Proposed: proposed, OutputNames: []string{}, Complete: true}
	report.Applyable = doc.Applyable != nil && *doc.Applyable
	for name, c := range doc.Outputs {
		if active(c.Actions) || unknown(c.AfterUnknown) {
			report.OutputNames = append(report.OutputNames, name)
		}
		if unknown(c.AfterUnknown) {
			report.OutputsUnknown = true
		}
	}
	sort.Strings(report.OutputNames)
	resources := append([]resourceChange(nil), doc.Resources...)
	externalRaw := append([]resourceChange(nil), doc.Drift...)
	sort.Slice(resources, func(i, j int) bool { return resources[i].Address < resources[j].Address })
	sort.Slice(externalRaw, func(i, j int) bool { return externalRaw[i].Address < externalRaw[j].Address })
	canonical, err := json.Marshal(struct {
		Resources, Drift []resourceChange
		Outputs          map[string]change
	}{resources, externalRaw, doc.Outputs})
	if err != nil {
		return Report{}, ErrPlan
	}
	sum := sha256.Sum256(canonical)
	report.PrivateChangeDigest = hex.EncodeToString(sum[:])
	return report, nil
}

// InPlaceOnly is a preliminary gate. State identity and consumer output pins
// must still be validated by repair admission under the original state lock.
func (r Report) InPlaceOnly() bool {
	if !r.Complete || !r.Applyable || r.OutputsUnknown || len(r.OutputNames) > 0 || len(r.Proposed) == 0 {
		return false
	}
	for _, f := range r.External {
		if len(f.Actions) != 1 || f.Actions[0] != "update" {
			return false
		}
	}
	for _, f := range r.Proposed {
		if len(f.Actions) != 1 || f.Actions[0] != "update" {
			return false
		}
		for _, p := range f.Paths {
			if p == "/<import>" || p == "/<identity>" {
				return false
			}
		}
	}
	return true
}
