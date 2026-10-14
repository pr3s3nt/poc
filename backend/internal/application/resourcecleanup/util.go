package resourcecleanup

import "sort"

func sortStrings(in []string) { sort.Strings(in) }

// sortIDs orders resource IDs root first, then by descriptor, then by ID.
func sortIDs(in []string, snap *snapshot, root string) {
	sort.SliceStable(in, func(i, j int) bool {
		if (in[i] == root) != (in[j] == root) {
			return in[i] == root
		}
		di, dj := snap.records[in[i]].Instance.Identity.Descriptor, snap.records[in[j]].Instance.Identity.Descriptor
		if di != dj {
			return di < dj
		}
		return in[i] < in[j]
	})
}
