package v03run

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"orchestrator/test/integration/v2run"
)

// ReapResult is the truthful outcome of one cleanup attempt.
type ReapResult struct {
	// Owned is every namespace proven run-owned (name, UID, exact tags), merged
	// from the scenario's earlier record and the product receipts.
	Owned []OwnedNamespace `json:"owned"`
	// Deleted are the namespaces this attempt asked the API server to delete.
	Deleted []string `json:"deleted"`
	// Absent are the run namespaces that did not exist when they were looked at.
	Absent []string `json:"absent"`
	// Unproven are run-named namespaces that exist but could not be proven run-owned;
	// they are never deleted and make the cleanup a failure.
	Unproven []string `json:"unproven"`
	// Remaining are run namespaces still present after the wait.
	Remaining []string `json:"remaining"`
	Problems  []string `json:"problems"`
}

// OK reports a fully proven cleanup: nothing unproven, nothing remaining, no problem.
func (r ReapResult) OK() bool {
	return len(r.Unproven) == 0 && len(r.Remaining) == 0 && len(r.Problems) == 0
}

// Reap removes exactly the namespaces proven run-owned, whatever state the run
// died in (partial creation included), and verifies the cluster afterwards.
// prior is the scenario's own earlier record; receipts are the product's
// Console receipts (nil when the database cannot be read). A namespace that
// exists but is covered by neither proof is reported Unproven and left alone.
func Reap(ctx context.Context, k v2run.Kubectl, tmpDir string, base Baseline, runID string, prior []OwnedNamespace, receipts []Receipt, wait time.Duration) ReapResult {
	var result ReapResult
	problem := func(format string, args ...any) {
		result.Problems = append(result.Problems, fmt.Sprintf(format, args...))
	}

	allowed := map[string]bool{}
	for _, name := range base.RunNamespaces {
		allowed[name] = true
	}
	owned := map[string]OwnedNamespace{}
	for _, record := range prior {
		if !allowed[record.Name] {
			problem("the recorded namespace %s is not one of the baseline run namespaces", record.Name)
			continue
		}
		if record.UID == "" || record.RunID != runID {
			problem("the record of %s is incomplete or belongs to another run", record.Name)
			continue
		}
		owned[record.Name] = record
	}
	fresh, absent, err := RecordOwned(ctx, k, base, runID, receipts, base.RunNamespaces)
	result.Absent = absent
	if err != nil && fresh == nil && absent == nil {
		problem("%v", err)
	}
	for _, record := range fresh {
		if had, ok := owned[record.Name]; ok && had.UID != record.UID {
			problem("namespace %s carries a different UID than the scenario recorded; it is not deleted", record.Name)
			delete(owned, record.Name)
			result.Unproven = append(result.Unproven, record.Name)
			continue
		}
		owned[record.Name] = record
	}
	current, listErr := namespacesFull(ctx, k)
	if listErr != nil {
		problem("%v", listErr)
		result.Owned = sortedOwned(owned)
		return result
	}
	for _, name := range base.RunNamespaces {
		got, present := current[name]
		if !present {
			continue
		}
		record, proven := owned[name]
		if !proven {
			result.Unproven = append(result.Unproven, name)
			continue
		}
		if got.UID != record.UID {
			// A namespace of the same name but another UID is a replacement, never ours.
			result.Unproven = append(result.Unproven, name)
			delete(owned, name)
			continue
		}
		deleted, err := GuardDelete(ctx, k, tmpDir, record)
		if err != nil {
			problem("namespace %s: %v", name, err)
			continue
		}
		if deleted {
			result.Deleted = append(result.Deleted, name)
		}
	}
	result.Owned = sortedOwned(owned)

	deadline := time.Now().Add(wait)
	for {
		now, err := namespacesFull(ctx, k)
		if err != nil {
			problem("%v", err)
			break
		}
		result.Remaining = result.Remaining[:0]
		for _, name := range base.RunNamespaces {
			if _, still := now[name]; still && !contains(result.Unproven, name) {
				result.Remaining = append(result.Remaining, name)
			}
		}
		if len(result.Remaining) == 0 || !time.Now().Before(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			problem("cleanup was interrupted")
			result.Remaining = append([]string(nil), result.Remaining...)
			return result
		case <-time.After(2 * time.Second):
		}
	}
	if err := VerifyAbsent(ctx, k, base, nil); err != nil {
		// Unproven namespaces are expected to remain; only baseline damage is a separate problem.
		for _, part := range strings.Split(err.Error(), "; ") {
			if strings.HasPrefix(part, "baseline namespace") {
				problem("%s", part)
			}
		}
	}
	sort.Strings(result.Unproven)
	return result
}

func contains(list []string, name string) bool {
	for _, item := range list {
		if item == name {
			return true
		}
	}
	return false
}

func sortedOwned(m map[string]OwnedNamespace) []OwnedNamespace {
	out := make([]OwnedNamespace, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
