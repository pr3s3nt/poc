package fleet

import "testing"

func TestControllerReadinessCannotUsePreviousCommit(t *testing.T) {
	object := func(commit string) map[string]any {
		return map[string]any{"metadata": map[string]any{"labels": map[string]any{"fleet.cattle.io/commit": commit}}, "status": map[string]any{"ready": true, "appliedDeploymentID": "old-id"}}
	}
	if controllerCommitMatches("new-commit", object("old-commit"), object("old-commit")) {
		t.Fatal("old ready controllers admitted after GitRepo commit advanced")
	}
	if controllerCommitMatches("new-commit", object("new-commit"), object("old-commit")) {
		t.Fatal("Bundle advancement admitted an old BundleDeployment")
	}
	if controllerCommitMatches("", object(""), object("")) {
		t.Fatal("missing commit admitted")
	}
	if !controllerCommitMatches("new-commit", object("new-commit"), object("new-commit")) {
		t.Fatal("matching controller chain rejected")
	}
}
