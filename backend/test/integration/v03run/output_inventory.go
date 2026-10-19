package v03run

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
)

// ListOutputRefs inventories metadata only, never secret values. Unexpected
// paths, denied reads and malformed/partial listings are errors, not emptiness.
func (v Vault) ListOutputRefs(ctx context.Context) ([]string, error) {
	list := func(path string) ([]string, error) {
		status, body, err := v.do(ctx, "LIST", "metadata/orchestrator/outputs/"+path)
		if err != nil {
			return nil, err
		}
		if status == http.StatusNotFound {
			return []string{}, nil
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("output metadata listing failed (HTTP %d)", status)
		}
		var doc struct {
			Data struct {
				Keys []string `json:"keys"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &doc) != nil || doc.Data.Keys == nil {
			return nil, fmt.Errorf("output metadata listing is malformed")
		}
		return doc.Data.Keys, nil
	}
	groups, err := list("")
	if err != nil {
		return nil, err
	}
	groupPattern := regexp.MustCompile(`^[0-9a-f]{16}/$`)
	refs := []string{}
	for _, group := range groups {
		if !groupPattern.MatchString(group) {
			return nil, fmt.Errorf("unexpected output metadata group")
		}
		keys, err := list(group)
		if err != nil {
			return nil, err
		}
		// A listed group disappearing mid-inventory cannot prove completeness.
		if len(keys) == 0 {
			return nil, fmt.Errorf("output metadata group disappeared during inventory")
		}
		for _, key := range keys {
			if !objectID.MatchString(key) {
				return nil, fmt.Errorf("unexpected output metadata object")
			}
			refs = append(refs, "secret://vault/"+v.Mount+"/orchestrator/outputs/"+group+key)
		}
	}
	sort.Strings(refs)
	for i := 1; i < len(refs); i++ {
		if refs[i] == refs[i-1] {
			return nil, fmt.Errorf("duplicate output metadata object")
		}
	}
	return refs, nil
}

// CheckOutputInventory protects baseline objects and rejects unaccounted new
// objects before SQL/recovery removal. Cleanup retries may already have deleted
// some inventoried owned refs; baseline objects must always remain unchanged.
func CheckOutputInventory(baseline, live, owned []string, afterClean bool) error {
	before, current, expected := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, ref := range baseline {
		before[ref] = true
	}
	for _, ref := range owned {
		if before[ref] {
			return fmt.Errorf("owned inventory overlaps protected baseline")
		}
		expected[ref] = true
	}
	for _, ref := range live {
		current[ref] = true
		if !before[ref] && (afterClean || !expected[ref]) {
			return fmt.Errorf("unaccounted output object remains; SQL and recovery material must be retained")
		}
	}
	for ref := range before {
		if !current[ref] {
			return fmt.Errorf("protected baseline output object changed")
		}
	}
	return nil
}
