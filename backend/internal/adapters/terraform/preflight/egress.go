// Package preflight holds the pure checks a live run makes before any effect.
// It never contacts the network: the caller supplies the executor egress
// address it observed. Wiring the observation into the runner belongs to the
// integration task (I04-10); a module validator cannot prove connectivity.
package preflight

import (
	"errors"
	"fmt"
	"net/netip"
)

// ErrEgressNotAllowed means the executor egress address is unknown or is not in
// the EKS public API allowlist. The message never echoes an address.
var ErrEgressNotAllowed = errors.New("preflight: the executor egress address is not in the EKS public API allowlist")

// CheckEgress returns nil only when egress is a valid IPv4 address and one of
// the allowlist entries is exactly egress/32. An empty, unparsable, IPv6 or
// unknown egress address, an empty allowlist, and any entry that is not a
// valid IPv4 /32 host (including a wider prefix or 0.0.0.0/0) fail closed. The
// list is never widened or extended.
func CheckEgress(egress string, allowlist []string) error {
	addr, err := netip.ParseAddr(egress)
	if err != nil || !addr.Is4() || addr.IsUnspecified() {
		return ErrEgressNotAllowed
	}
	if len(allowlist) == 0 {
		return ErrEgressNotAllowed
	}
	found := false
	for _, entry := range allowlist {
		prefix, err := netip.ParsePrefix(entry)
		if err != nil || !prefix.Addr().Is4() || prefix.Bits() != 32 || prefix.Addr().IsUnspecified() || prefix.Masked() != prefix {
			return fmt.Errorf("%w (invalid allowlist entry)", ErrEgressNotAllowed)
		}
		if prefix.Addr() == addr {
			found = true
		}
	}
	if !found {
		return ErrEgressNotAllowed
	}
	return nil
}
