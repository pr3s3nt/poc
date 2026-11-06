package workloadspec

import (
	"encoding/json"
	"fmt"
)

// New optional fields use omission for inheritance; explicit null is not silently
// normalized to absence. This avoids accepting imports that cannot round-trip.
func validateRuntimeFields(b []byte) error {
	var raw struct {
		Containers map[string]map[string]json.RawMessage `json:"containers"`
		Execution  map[string]json.RawMessage            `json:"workloadExecution"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	check := func(obj map[string]json.RawMessage, keys ...string) error {
		for _, key := range keys {
			if v, ok := obj[key]; ok && string(v) == "null" {
				return fmt.Errorf("workload: %s must be omitted to inherit", key)
			}
		}
		return nil
	}
	for _, c := range raw.Containers {
		if err := check(c, "securityContext"); err != nil {
			return err
		}
		if b, ok := c["securityContext"]; ok {
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(b, &fields)
			for key := range fields {
				if err := check(fields, key); err != nil {
					return err
				}
			}
		}
	}
	for _, key := range []string{"defaults", "podSecurityContext", "automountServiceAccountToken", "serviceAccountName", "serviceEnabled", "updateStrategy", "revisionHistoryLimit", "podDisruptionBudget", "volumeClaimTemplates", "claimMounts", "persistentVolumeClaimRetentionPolicy"} {
		if err := check(raw.Execution, key); err != nil {
			return err
		}
		if b, ok := raw.Execution[key]; ok {
			var fields map[string]json.RawMessage
			if json.Unmarshal(b, &fields) == nil {
				for k := range fields {
					if err := check(fields, k); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
