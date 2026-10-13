package preflight

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckEgress(t *testing.T) {
	list := []string{"203.0.113.10/32", "198.51.100.7/32"}
	if err := CheckEgress("198.51.100.7", list); err != nil {
		t.Fatalf("listed host: %v", err)
	}
	for name, c := range map[string]struct {
		egress string
		list   []string
	}{
		"unknown egress":     {"", list},
		"unlisted":           {"192.0.2.1", list},
		"ipv6":               {"2001:db8::1", list},
		"unspecified":        {"0.0.0.0", list},
		"garbage":            {"not-an-ip", list},
		"empty list":         {"203.0.113.10", nil},
		"open to all":        {"203.0.113.10", []string{"0.0.0.0/0"}},
		"wide prefix":        {"203.0.113.10", []string{"203.0.113.0/24"}},
		"entry without mask": {"203.0.113.10", []string{"203.0.113.10"}},
		"bad entry mixed in": {"203.0.113.10", []string{"203.0.113.10/32", "999.1.1.1/32"}},
	} {
		err := CheckEgress(c.egress, c.list)
		if !errors.Is(err, ErrEgressNotAllowed) {
			t.Errorf("%s: want ErrEgressNotAllowed, got %v", name, err)
			continue
		}
		if strings.Contains(err.Error(), "203.0.113") || strings.Contains(err.Error(), "192.0.2") {
			t.Errorf("%s: error echoes an address", name)
		}
	}
}
