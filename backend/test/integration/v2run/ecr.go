package v2run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var ecrNotFound = regexp.MustCompile(`(?i)RepositoryNotFoundException|repository.*not found`)

// DockerCLI is the local, pinned image source used to publish immutable AWS
// acceptance images. Input/output may carry the short-lived ECR login token;
// implementations must never log it.
type DockerCLI interface {
	Run(ctx context.Context, input []byte, args ...string) (stdout []byte, stderr string, exit int)
}

// ExecDocker invokes the configured Docker client without a shell.
type ExecDocker struct {
	Binary    string
	ConfigDir string
}

func (d ExecDocker) Run(ctx context.Context, input []byte, args ...string) ([]byte, string, int) {
	cmd := exec.CommandContext(ctx, d.Binary, args...)
	cmd.Env = baseEnv()
	if d.ConfigDir != "" {
		cmd.Env = append(cmd.Env, "DOCKER_CONFIG="+d.ConfigDir)
	}
	cmd.Stdin = strings.NewReader(string(input))
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if err == nil {
		return []byte(out.String()), errOut.String(), 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return []byte(out.String()), errOut.String(), ee.ExitCode()
	}
	return []byte(out.String()), errOut.String(), 1
}

// ECRPublication is the reviewed output passed to the Console workload forms.
type ECRPublication struct {
	Images       map[string]string `json:"images"`
	Repositories []string          `json:"repositories"` // exact repository ARNs, sorted
}

// ECRRepositoryNames returns the deterministic, per-run repository names.
func ECRRepositoryNames(c Config) map[string]string {
	out := make(map[string]string, 3)
	for _, workload := range []string{"frontend", "backend", "worker"} {
		out[workload] = c.AWS.ECRRepositoryPrefix + "-" + c.RunID + "-" + workload
	}
	return out
}

// PrepareECRImages creates exact run-owned immutable ECR repositories, records
// each name before creation, pushes only the selected architecture's pinned
// local image and verifies the registry's immutable manifest digest.
func PrepareECRImages(ctx context.Context, aws AWSCLI, docker DockerCLI, c Config, manifestPath string) (ECRPublication, error) {
	var result ECRPublication
	if c.Target != TargetAWS || c.AWS == nil || !IsAWSScenario(c.Scenario) {
		return result, errors.New("ECR image preparation requires an AWS acceptance run")
	}
	m, err := LoadManifest(manifestPath)
	if err != nil || m.RunID != c.RunID || !m.Armed {
		return result, errors.New("an armed private manifest for this run is required")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	names := ECRRepositoryNames(c)
	for _, workload := range []string{"frontend", "backend", "worker"} {
		name := names[workload]
		_, stderr, code := aws.Run(ctx, "ecr", "describe-repositories", "--repository-names", name)
		if code == 0 {
			return result, fmt.Errorf("the deterministic ECR repository %s already exists; refusing to claim it", name)
		}
		if !ecrNotFound.MatchString(stderr) {
			return result, fmt.Errorf("the ECR repository %s absence is indeterminate", name)
		}
		if _, _, code := docker.Run(ctx, nil, "image", "inspect", "--format={{.Id}}|{{.Architecture}}", c.Images[c.NodeArch][workload].Ref); code != 0 {
			return result, fmt.Errorf("the pinned local %s image is unavailable", workload)
		}
	}
	// Planned ownership entries are durable before the first create API call.
	if err := UpdateManifest(manifestPath, func(m *Manifest) error {
		for _, workload := range []string{"frontend", "backend", "worker"} {
			if err := m.AddAsset(Asset{Kind: "ecr-repository", ID: names[workload], Layer: LayerAux, Owner: OwnerCreatedByRun}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return result, fmt.Errorf("cannot record ECR ownership before create: %w", err)
	}
	for _, workload := range []string{"frontend", "backend", "worker"} {
		image := c.Images[c.NodeArch][workload]
		out, _, code := docker.Run(ctx, nil, "image", "inspect", "--format={{.Id}}|{{.Architecture}}", image.Ref)
		imageID, arch, found := strings.Cut(strings.TrimSpace(string(out)), "|")
		if code != 0 || !found || image.ImageID == "" || imageID != image.ImageID || arch != c.NodeArch {
			return result, fmt.Errorf("the local %s image digest or architecture does not match its pin", workload)
		}
	}
	login, _, code := aws.Run(ctx, "ecr", "get-login-password")
	if code != 0 || len(strings.TrimSpace(string(login))) == 0 {
		return result, errors.New("ECR login token could not be obtained")
	}
	registry := c.AWS.AccountID + ".dkr.ecr." + c.AWS.Region + ".amazonaws.com"
	if _, _, code := docker.Run(ctx, login, "login", "--username", "AWS", "--password-stdin", registry); code != 0 {
		return result, errors.New("Docker could not authenticate to the run's ECR registry")
	}
	result.Images = map[string]string{}
	for _, workload := range []string{"frontend", "backend", "worker"} {
		name := names[workload]
		image := c.Images[c.NodeArch][workload]
		tagged := registry + "/" + name + ":" + c.RunID
		createTags := []string{"--tags", "Key=project,Value=orchestrator", "Key=owner,Value=" + c.Owner, "Key=environment,Value=" + ProductEnvironmentTag,
			"Key=run-id,Value=" + c.RunID, "Key=expires-at,Value=" + m.ExpiresAt.UTC().Format(time.RFC3339), "Key=managed-by,Value=" + ProductManagedByTag}
		repoRaw, repoErr, repoCode := aws.Run(ctx, append([]string{"ecr", "create-repository", "--repository-name", name, "--image-tag-mutability", "IMMUTABLE"}, createTags...)...)
		if repoCode != 0 {
			return result, fmt.Errorf("the pre-recorded ECR repository %s could not be created: %s", name, firstLine(repoErr))
		}
		var created struct {
			Repository struct {
				ARN            string `json:"repositoryArn"`
				RepositoryURI  string `json:"repositoryUri"`
				RepositoryName string `json:"repositoryName"`
			}
		}
		if json.Unmarshal(repoRaw, &created) != nil || created.Repository.RepositoryName != name || created.Repository.RepositoryURI != registry+"/"+name || created.Repository.ARN != ecrRepositoryARN(c, name) {
			return result, fmt.Errorf("ECR returned an unexpected physical repository for %s", name)
		}
		if err := UpdateManifest(manifestPath, func(m *Manifest) error {
			return m.AddAsset(Asset{Kind: "ecr-repository", ID: name, Layer: LayerAux, Owner: OwnerCreatedByRun, Linkage: created.Repository.ARN})
		}); err != nil {
			return result, fmt.Errorf("ECR created %s but its physical ARN was not acknowledged; cleanup is held: %w", name, err)
		}
		tagsRaw, tagErr, tagCode := aws.Run(ctx, "ecr", "list-tags-for-resource", "--resource-arn", created.Repository.ARN)
		if tagCode != 0 || !exactECRTags(tagsRaw, m.Tags) {
			return result, fmt.Errorf("the ECR repository %s ownership tags do not match the run: %s", name, firstLine(tagErr))
		}
		if _, _, code := docker.Run(ctx, nil, "tag", image.Ref, tagged); code != 0 {
			return result, fmt.Errorf("the pinned %s image could not be tagged for ECR", workload)
		}
		if _, _, code := docker.Run(ctx, nil, "push", tagged); code != 0 {
			return result, fmt.Errorf("the pinned %s image could not be pushed to its run-owned ECR repository", workload)
		}
		pushed, pushErr, pushCode := aws.Run(ctx, "ecr", "describe-images", "--repository-name", name, "--image-ids", "imageTag="+c.RunID)
		var details struct {
			ImageDetails []struct{ ImageDigest string }
		}
		if pushCode != 0 || json.Unmarshal(pushed, &details) != nil || len(details.ImageDetails) != 1 || details.ImageDetails[0].ImageDigest != image.Digest {
			return result, fmt.Errorf("the pushed %s image digest did not match the immutable pin: %s", workload, firstLine(pushErr))
		}
		result.Images[workload] = created.Repository.RepositoryURI + "@" + image.Digest
		result.Repositories = append(result.Repositories, created.Repository.ARN)
	}
	return result, nil
}

// CleanupECRRepositories only deletes exact manifest-listed repositories whose
// account, region, ARN and full run tag set still prove CREATED_BY_RUN.
func CleanupECRRepositories(ctx context.Context, aws AWSCLI, c Config, manifestPath string, verifyOnly bool) error {
	m, err := LoadManifest(manifestPath)
	if err != nil || m.RunID != c.RunID || m.Target != TargetAWS {
		return errors.New("the private AWS ownership manifest cannot be verified")
	}
	for _, asset := range m.Assets {
		if asset.Kind != "ecr-repository" || asset.Owner != OwnerCreatedByRun {
			continue
		}
		expectedARN := ecrRepositoryARN(c, asset.ID)
		if asset.ID == "" || asset.ID != ECRRepositoryNames(c)["frontend"] && asset.ID != ECRRepositoryNames(c)["backend"] && asset.ID != ECRRepositoryNames(c)["worker"] {
			return fmt.Errorf("unrecognized ECR repository asset %q; ownership is held", asset.ID)
		}
		repoRaw, stderr, code := aws.Run(ctx, "ecr", "describe-repositories", "--repository-names", asset.ID)
		if code != 0 {
			if ecrNotFound.MatchString(stderr) {
				continue
			}
			return fmt.Errorf("the exact ECR repository %s absence is indeterminate", asset.ID)
		}
		var described struct {
			Repositories []struct {
				ARN            string `json:"repositoryArn"`
				RepositoryName string `json:"repositoryName"`
			}
		}
		if json.Unmarshal(repoRaw, &described) != nil || len(described.Repositories) != 1 || described.Repositories[0].RepositoryName != asset.ID || described.Repositories[0].ARN != expectedARN || asset.Linkage != "" && asset.Linkage != expectedARN {
			return fmt.Errorf("the ECR repository %s physical identity differs from its ownership record", asset.ID)
		}
		tagsRaw, tagErr, tagCode := aws.Run(ctx, "ecr", "list-tags-for-resource", "--resource-arn", expectedARN)
		var tagDoc struct{ Tags []struct{ Key, Value string } }
		if tagCode != 0 || json.Unmarshal(tagsRaw, &tagDoc) != nil {
			return fmt.Errorf("the ECR repository %s ownership tags are indeterminate: %s", asset.ID, firstLine(tagErr))
		}
		tags := map[string]string{}
		for _, tag := range tagDoc.Tags {
			tags[tag.Key] = tag.Value
		}
		if len(tags) != len(m.Tags) {
			return fmt.Errorf("the ECR repository %s ownership tag set changed; it is preserved", asset.ID)
		}
		for key, want := range m.Tags {
			if tags[key] != want {
				return fmt.Errorf("the ECR repository %s ownership tag %s changed; it is preserved", asset.ID, key)
			}
		}
		if asset.Linkage == "" {
			if err := UpdateManifest(manifestPath, func(m *Manifest) error {
				return m.AddAsset(Asset{Kind: asset.Kind, ID: asset.ID, Layer: LayerAux, Owner: OwnerCreatedByRun, Linkage: expectedARN})
			}); err != nil {
				return fmt.Errorf("the ECR repository %s ARN could not be acknowledged; it is preserved", asset.ID)
			}
		}
		if !verifyOnly {
			_, deleteErr, deleteCode := aws.Run(ctx, "ecr", "delete-repository", "--repository-name", asset.ID, "--force")
			if deleteCode != 0 && !ecrNotFound.MatchString(deleteErr) {
				return fmt.Errorf("the exact run-owned ECR repository %s could not be deleted", asset.ID)
			}
			after, afterErr, afterCode := aws.Run(ctx, "ecr", "describe-repositories", "--repository-names", asset.ID)
			if afterCode == 0 || !ecrNotFound.MatchString(afterErr) || len(after) != 0 {
				return fmt.Errorf("the exact run-owned ECR repository %s is not verified absent", asset.ID)
			}
		}
	}
	return nil
}

func ecrRepositoryARN(c Config, name string) string {
	return "arn:aws:ecr:" + c.AWS.Region + ":" + c.AWS.AccountID + ":repository/" + name
}

func exactECRTags(raw []byte, want map[string]string) bool {
	var doc struct{ Tags []struct{ Key, Value string } }
	if json.Unmarshal(raw, &doc) != nil || len(doc.Tags) != len(want) {
		return false
	}
	for _, tag := range doc.Tags {
		if want[tag.Key] != tag.Value {
			return false
		}
	}
	return true
}
