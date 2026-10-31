package catalogrelease

import (
	"strings"
	"testing"
)

const validManifest = `apiVersion: orchestrator.dev/catalog/v1
components:
- id: default
  kind: definition
  path: definitions/default.yaml
- id: database
  kind: terraform-module
  path: modules/postgres
  resourceType: postgres
`

func TestManifestStrictness(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	if err != nil || len(m.Components) != 2 || m.Components[0].ID != "database" {
		t.Fatal(m, err)
	}
	for name, raw := range map[string]string{
		"unknown":         validManifest + "unexpected: true\n",
		"version":         strings.Replace(validManifest, APIVersion, "v2", 1),
		"traversal":       strings.Replace(validManifest, "modules/postgres", "../postgres", 1),
		"absolute":        strings.Replace(validManifest, "modules/postgres", "/postgres", 1),
		"git path":        strings.Replace(validManifest, "modules/postgres", ".git/objects", 1),
		"duplicate id":    strings.Replace(validManifest, "id: database", "id: default", 1),
		"duplicate path":  strings.Replace(validManifest, "modules/postgres", "definitions/default.yaml", 1),
		"duplicate key":   validManifest + "apiVersion: other\n",
		"second document": validManifest + "---\n{}\n",
		"alias":           strings.Replace(validManifest, "- id: database", "- &alias\n  id: database", 1),
		"missing type":    strings.Replace(validManifest, "  resourceType: postgres\n", "", 1),
		"oversized":       strings.Repeat("x", MaxManifestBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest([]byte(raw)); err != ErrInvalid {
				t.Fatal("unsafe manifest accepted", err)
			}
		})
	}
}
func TestSourceGuards(t *testing.T) {
	s := Source{Organization: "org", ID: "main", Repository: "https://example.com/catalog.git", Branch: "main", TagPattern: "v*", ManifestPath: "catalog.yaml", Version: 1}
	if s.Validate() != nil {
		t.Fatal("valid source rejected")
	}
	for _, branch := range []string{"--all", "refs/heads/main", "main.lock", "a/.b", "a..b", "main/", "@{x}"} {
		bad := s
		bad.Branch = branch
		if bad.Validate() == nil {
			t.Fatalf("accepted %s", branch)
		}
	}
	for _, url := range []string{"http://example.com/x", "https://user:secret@example.com/x", "https://example.com/x?token=secret", "https://example.com/x#ref"} {
		bad := s
		bad.Repository = url
		if bad.Validate() == nil {
			t.Fatal("unsafe URL accepted")
		}
	}
}
func TestComponentDiffStable(t *testing.T) {
	before := Release{Components: []VerifiedComponent{{Component: Component{ID: "same", Kind: TerraformModule}, SHA256: "a"}, {Component: Component{ID: "updated", Kind: RenderBundle}, SHA256: "a"}, {Component: Component{ID: "removed"}}}}
	after := Release{Components: []VerifiedComponent{{Component: Component{ID: "same", Kind: TerraformModule}, SHA256: "a"}, {Component: Component{ID: "updated", Kind: RenderBundle}, SHA256: "b"}, {Component: Component{ID: "added"}}}}
	got := Compare(before, after)
	want := []Difference{{"added", "added"}, {"removed", "removed"}, {"same", "no-change"}, {"updated", "update"}}
	for i := range want {
		if got[i] != want[i] {
			t.Fatal(got)
		}
	}
}
func TestDefinitionRejectsTransportAndAccountPins(t *testing.T) {
	raw := `{"binding":"internal","definition":{"key":"cluster","name":"Cluster","resourceType":"k8s-cluster","driverType":"existing-cluster","version":1,"criteria":[{}]}}`
	if _, err := ParseDefinition([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(raw, `"name":"Cluster"`, `"name":"Cluster","driverAccount":"other-org"`, 1), strings.Replace(raw, `"binding":"internal"`, `"binding":"internal","unknown":true`, 1), raw + "\n---\n{}"} {
		if _, err := ParseDefinition([]byte(bad)); err != ErrInvalid {
			t.Fatal("unsafe definition accepted", err)
		}
	}
}
func TestReleaseOrderIsNumeric(t *testing.T) {
	releases := []Release{{Tag: "v1.9.0"}, {Tag: "v1.10.0-rc.1"}, {Tag: "v1.10.0"}}
	SortReleases(releases)
	if releases[0].Tag != "v1.10.0" || releases[1].Tag != "v1.10.0-rc.1" || releases[2].Tag != "v1.9.0" {
		t.Fatal(releases)
	}
}
