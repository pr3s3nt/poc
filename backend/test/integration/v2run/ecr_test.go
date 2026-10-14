package v2run_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"orchestrator/test/integration/v2run"
)

type ecrAWS struct {
	repos        map[string]string
	tags         map[string]map[string]string
	expectedTags map[string]string
	deleted      []string
}

func (f *ecrAWS) Run(_ context.Context, a ...string) ([]byte, string, int) {
	if len(a) < 2 {
		return nil, "bad command", 2
	}
	switch a[0] + " " + a[1] {
	case "ecr describe-repositories":
		name := a[3]
		arn, ok := f.repos[name]
		if !ok {
			return nil, "RepositoryNotFoundException", 254
		}
		b, _ := json.Marshal(map[string]any{"repositories": []any{map[string]string{"repositoryArn": arn, "repositoryName": name}}})
		return b, "", 0
	case "ecr get-login-password":
		return []byte("synthetic-token\n"), "", 0
	case "ecr create-repository":
		name := a[3]
		arn := "arn:aws:ecr:us-east-1:452025861381:repository/" + name
		f.repos[name] = arn
		f.tags[name] = map[string]string{}
		for k, v := range f.expectedTags {
			f.tags[name][k] = v
		}
		b, _ := json.Marshal(map[string]any{"repository": map[string]string{"repositoryArn": arn, "repositoryName": name, "repositoryUri": "452025861381.dkr.ecr.us-east-1.amazonaws.com/" + name}})
		return b, "", 0
	case "ecr list-tags-for-resource":
		arn := a[3]
		name := strings.TrimPrefix(arn, "arn:aws:ecr:us-east-1:452025861381:repository/")
		tags := []map[string]string{}
		for k, v := range f.tags[name] {
			tags = append(tags, map[string]string{"key": k, "value": v})
		}
		b, _ := json.Marshal(map[string]any{"tags": tags})
		return b, "", 0
	case "ecr describe-images":
		b, _ := json.Marshal(map[string]any{"imageDetails": []any{map[string]string{"imageDigest": "sha256:" + strings.Repeat("b", 64)}}})
		return b, "", 0
	case "ecr delete-repository":
		name := a[3]
		f.deleted = append(f.deleted, name)
		delete(f.repos, name)
		delete(f.tags, name)
		return []byte(`{"repository":{}}`), "", 0
	}
	return nil, "unexpected", 255
}

type ecrDocker struct {
	refs  map[string]string
	calls [][]string
}

func (f *ecrDocker) Run(_ context.Context, input []byte, a ...string) ([]byte, string, int) {
	f.calls = append(f.calls, append([]string(nil), a...))
	switch a[0] {
	case "image":
		if a[1] == "inspect" {
			ref := a[len(a)-1]
			return []byte(f.refs[ref] + "|arm64\n"), "", 0
		}
	case "login":
		if string(input) != "synthetic-token\n" {
			return nil, "missing stdin token", 1
		}
		return nil, "", 0
	case "tag", "push":
		return nil, "", 0
	}
	return nil, "unexpected", 255
}

func TestPrepareAndCleanupECRRepositoriesArePreRecordedAndExact(t *testing.T) {
	c := awsConfig(t, "A02", "t4g.medium")
	c.RunID = "a02-0001"
	for _, w := range []string{"frontend", "backend", "worker"} {
		c.Images["arm64"][w] = v2run.Image{Ref: "local/" + w + ":pin", Digest: "sha256:" + strings.Repeat("b", 64), ImageID: "sha256:" + strings.Repeat("c", 64)}
	}
	m := v2run.NewManifest(c, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))
	m.Armed = true
	path := t.TempDir() + "/manifest.json"
	if err := v2run.SaveManifest(path, m); err != nil {
		t.Fatal(err)
	}
	docker := &ecrDocker{refs: map[string]string{}}
	for _, im := range c.Images["arm64"] {
		docker.refs[im.Ref] = im.ImageID
	}
	aws := &ecrAWS{repos: map[string]string{}, tags: map[string]map[string]string{}, expectedTags: m.Tags}
	pub, err := v2run.PrepareECRImages(context.Background(), aws, docker, c, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(pub.Images) != 3 || len(pub.Repositories) != 3 {
		t.Fatalf("incomplete publication: %+v", pub)
	}
	manifest, err := v2run.LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, workload := range []string{"frontend", "backend", "worker"} {
		name := v2run.ECRRepositoryNames(c)[workload]
		found := false
		for _, asset := range manifest.Assets {
			if asset.ID == name && asset.Kind == "ecr-repository" && asset.Owner == v2run.OwnerCreatedByRun && asset.Linkage == aws.repos[name] {
				found = true
			}
		}
		if !found {
			t.Fatalf("repository %s wasn't acknowledged in manifest", name)
		}
		if !strings.HasSuffix(pub.Images[workload], "@sha256:"+strings.Repeat("b", 64)) {
			t.Fatalf("image isn't digest pinned: %s", pub.Images[workload])
		}
	}
	if err := v2run.CleanupECRRepositories(context.Background(), aws, c, path, false); err != nil {
		t.Fatal(err)
	}
	if len(aws.deleted) != 3 || len(aws.repos) != 0 {
		t.Fatalf("cleanup wasn't exact: %+v", aws.deleted)
	}
}

func TestCleanupECRPreservesChangedOwnershipTags(t *testing.T) {
	c := awsConfig(t, "A02", "t4g.medium")
	c.RunID = "a02-0001"
	m := v2run.NewManifest(c, time.Now())
	name := v2run.ECRRepositoryNames(c)["frontend"]
	arn := fmt.Sprintf("arn:aws:ecr:%s:%s:repository/%s", c.AWS.Region, c.AWS.AccountID, name)
	if err := m.AddAsset(v2run.Asset{Kind: "ecr-repository", ID: name, Layer: "aux", Owner: v2run.OwnerCreatedByRun, Linkage: arn}); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/manifest.json"
	if err := v2run.SaveManifest(path, m); err != nil {
		t.Fatal(err)
	}
	aws := &ecrAWS{repos: map[string]string{name: arn}, tags: map[string]map[string]string{name: {"run-id": "foreign"}}}
	if err := v2run.CleanupECRRepositories(context.Background(), aws, c, path, false); err == nil {
		t.Fatal("changed run tag was accepted")
	}
	if len(aws.deleted) != 0 {
		t.Fatalf("foreign repository was deleted: %v", aws.deleted)
	}
}
