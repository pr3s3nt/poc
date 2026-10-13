package registry

import (
	"errors"
	"strconv"
	"strings"
)

// version is a MAJOR.MINOR.PATCH release version.
type version [3]int

func parseVersion(text string, allowShort bool) (version, int, error) {
	parts := strings.Split(strings.TrimSpace(text), ".")
	if len(parts) == 0 || len(parts) > 3 || (!allowShort && len(parts) != 3) {
		return version{}, 0, errors.New("invalid version")
	}
	var v version
	for i, part := range parts {
		if part == "" || strings.TrimLeft(part, "0123456789") != "" || len(part) > 9 {
			return version{}, 0, errors.New("invalid version")
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return version{}, 0, errors.New("invalid version")
		}
		v[i] = n
	}
	return v, len(parts), nil
}

func (v version) compare(o version) int {
	for i := range v {
		switch {
		case v[i] < o[i]:
			return -1
		case v[i] > o[i]:
			return 1
		}
	}
	return 0
}

// satisfies evaluates a Terraform version constraint string (comma-separated
// =, !=, >, >=, <, <=, ~> clauses) against an exact release version.
// Pre-release versions and unknown operators are rejected, not ignored.
func satisfies(exact, constraints string) (bool, error) {
	v, _, err := parseVersion(exact, false)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(constraints) == "" {
		return true, nil
	}
	for _, clause := range strings.Split(constraints, ",") {
		clause = strings.TrimSpace(clause)
		op := "="
		for _, candidate := range []string{"~>", ">=", "<=", "!=", ">", "<", "="} {
			if strings.HasPrefix(clause, candidate) {
				op, clause = candidate, strings.TrimSpace(strings.TrimPrefix(clause, candidate))
				break
			}
		}
		want, parts, err := parseVersion(clause, true)
		if err != nil {
			return false, err
		}
		cmp := v.compare(want)
		ok := false
		switch op {
		case "=":
			ok = cmp == 0
		case "!=":
			ok = cmp != 0
		case ">":
			ok = cmp > 0
		case ">=":
			ok = cmp >= 0
		case "<":
			ok = cmp < 0
		case "<=":
			ok = cmp <= 0
		case "~>":
			// ~> X.Y.Z allows only Z to increase; ~> X.Y and ~> X allow only
			// the rightmost given part to increase.
			upper := want
			switch parts {
			case 3:
				upper = version{want[0], want[1] + 1, 0}
			case 2:
				upper = version{want[0] + 1, 0, 0}
			default:
				upper = version{want[0] + 1, 0, 0}
			}
			ok = cmp >= 0 && v.compare(upper) < 0
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}
