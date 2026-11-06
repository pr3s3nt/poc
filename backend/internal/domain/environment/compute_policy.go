package environment

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
)

var computeQuantity = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)([numkKMGTPE]|[KMGTPE]i)?$`)

// Bounded numeric syntax used by v1 profiles; legacy API quantities keep their
// historical admission path. Kubernetes performs its final quota/admission checks.
func policyQuantity(raw string) (float64, bool) {
	m := computeQuantity.FindStringSubmatch(raw)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	factor := map[string]float64{"": 1, "n": 1e-9, "u": 1e-6, "m": 1e-3, "k": 1e3, "K": 1e3, "M": 1e6, "G": 1e9, "T": 1e12, "P": 1e15, "E": 1e18, "Ki": 1024, "Mi": 1048576, "Gi": 1073741824, "Ti": 1099511627776, "Pi": 1125899906842624, "Ei": 1152921504606846976}[m[2]]
	v := n * factor
	return v, !math.IsInf(v, 0) && v >= 0
}
func (m ModuleSpec) ValidateComputePolicy() error {
	if m.Execution == nil || m.Execution.Defaults == nil {
		return nil
	}
	for _, c := range m.Containers {
		r := c.Resources
		if r == nil {
			continue
		}
		for _, dimension := range []string{"cpu", "memory"} {
			request, limit := "", ""
			if r.Requests != nil {
				if dimension == "cpu" {
					request = r.Requests.CPU
				} else {
					request = r.Requests.Memory
				}
			}
			if r.Limits != nil {
				if dimension == "cpu" {
					limit = r.Limits.CPU
				} else {
					limit = r.Limits.Memory
				}
			}
			a, b := 0.0, 0.0
			var ok bool
			if request != "" {
				a, ok = policyQuantity(request)
				if !ok {
					return fmt.Errorf("%w: invalid request", ErrExecution)
				}
			}
			if limit != "" {
				b, ok = policyQuantity(limit)
				if !ok || b < a {
					return fmt.Errorf("%w: limit must cover request", ErrExecution)
				}
			}
		}
	}
	return nil
}
