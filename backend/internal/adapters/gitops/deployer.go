// Package gitops delivers workload manifests through a Fleet GitRepo. It never
// commits Kubernetes Secret objects or registry credentials to Git.
package gitops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	k8s "orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/ports/execution"
)

var segment = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// Options names a pre-cloned writable repository and the Fleet GitRepo that
// watches it. The Git transport obtains its credentials from the host's local
// git credential helper; no token is passed to the process or stored in state.
type Options struct {
	RepoDir, Branch, GitRepoName, KubeContext, KubectlPath string
	RegistryHost, DockerConfigFile, PullSecretName         string
	Timeout                                                time.Duration
}

type revision struct{ commit, deploymentID string }

// Deployer is a process-local, serialized Git writer. The remote repository is
// the desired-state authority for workload manifests in this mode.
type Deployer struct {
	opts Options
	mu   sync.Mutex
	last map[string]revision
}

func New(opts Options) (*Deployer, error) {
	if opts.RepoDir == "" || opts.Branch == "" || opts.GitRepoName == "" || opts.KubeContext == "" || opts.RegistryHost == "" || opts.DockerConfigFile == "" || opts.PullSecretName == "" {
		return nil, fmt.Errorf("gitops: repository, branch, GitRepo, kind context, Harbor registry and pull-secret configuration are required")
	}
	if !segment.MatchString(opts.Branch) || !segment.MatchString(opts.GitRepoName) || !segment.MatchString(opts.PullSecretName) {
		return nil, fmt.Errorf("gitops: invalid branch, GitRepo or pull-secret name")
	}
	if opts.KubectlPath == "" {
		opts.KubectlPath = "kubectl"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Minute
	}
	if _, err := os.Stat(filepath.Join(opts.RepoDir, ".git")); err != nil {
		return nil, fmt.Errorf("gitops: repository is not a git worktree: %w", err)
	}
	return &Deployer{opts: opts, last: map[string]revision{}}, nil
}

func (d *Deployer) cli() k8s.CLI { return k8s.NewCLI(d.opts.KubectlPath, d.opts.KubeContext, "") }

var errFleetCredentialTarget = errors.New("gitops: Fleet GitRepo delivery does not support credential-backed connection targets; use direct delivery")

func (d *Deployer) scope(target execution.Target, workload string) (string, error) {
	// Fleet delivery is fixed to the configured cluster (ADR-007). A
	// credential-backed Connection target fails closed even if its selected
	// context has the same name; it never silently uses the seed context.
	if target.CredentialBacked() {
		return "", errFleetCredentialTarget
	}
	if target.Context != d.opts.KubeContext || target.Namespace == "" || target.Extra == nil {
		return "", fmt.Errorf("gitops: workload target is not the configured kind cluster")
	}
	app, env := target.Extra["application"], target.Extra["environment"]
	if !segment.MatchString(app) || (env != "staging" && env != "production") || !segment.MatchString(workload) || !segment.MatchString(target.Namespace) {
		return "", fmt.Errorf("gitops: invalid Application/Environment/workload scope")
	}
	return filepath.Join("applications", app, env, workload), nil
}

func (d *Deployer) Apply(ctx context.Context, target execution.Target, manifests []execution.Manifest) error {
	if target.CredentialBacked() {
		return errFleetCredentialTarget
	}
	var workload, deploymentID string
	var public []execution.Manifest
	var private []map[string]any
	for _, manifest := range manifests {
		if manifest.Kind == "Deployment" {
			workload = manifest.Name
			metadata, _ := manifest.Object["metadata"].(map[string]any)
			labels, _ := metadata["labels"].(map[string]any)
			deploymentID, _ = labels["orchestrator.io/deployment-id"].(string)
			if err := d.validateImages(manifest.Object); err != nil {
				return err
			}
		}
		if manifest.Namespace != target.Namespace || manifest.Object == nil {
			return fmt.Errorf("gitops: manifest is outside the target namespace")
		}
		if manifest.Secret || manifest.Kind == "Secret" {
			private = append(private, manifest.Object)
		} else {
			public = append(public, manifest)
		}
	}
	if workload == "" || deploymentID == "" {
		return fmt.Errorf("gitops: workload Deployment and revision label are required")
	}
	path, err := d.scope(target, workload)
	if err != nil {
		return err
	}
	if target.Extra["deployment"] != deploymentID {
		return fmt.Errorf("gitops: workload revision does not match deployment")
	}
	if err := d.ensurePullSecret(ctx, target.Namespace); err != nil {
		return err
	}
	if len(private) > 0 {
		if err := d.cli().Apply(ctx, private); err != nil {
			return fmt.Errorf("gitops: apply private runtime objects: %w", err)
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	commit, err := d.writeAndPush(ctx, path, target.Namespace, public, false)
	if err != nil {
		return err
	}
	d.last[path+"/"+deploymentID] = revision{commit: commit, deploymentID: deploymentID}
	return nil
}

func (d *Deployer) validateImages(object map[string]any) error {
	spec, _ := object["spec"].(map[string]any)
	template, _ := spec["template"].(map[string]any)
	pod, _ := template["spec"].(map[string]any)
	containers, _ := pod["containers"].([]any)
	if len(containers) == 0 {
		return fmt.Errorf("gitops: Deployment has no containers")
	}
	for _, raw := range containers {
		container, _ := raw.(map[string]any)
		image, _ := container["image"].(string)
		if !strings.HasPrefix(image, d.opts.RegistryHost+"/") {
			return fmt.Errorf("gitops: image must come from configured Harbor registry")
		}
	}
	return nil
}

func (d *Deployer) ensurePullSecret(ctx context.Context, namespace string) error {
	info, err := os.Stat(d.opts.DockerConfigFile)
	if err != nil {
		return fmt.Errorf("gitops: stat Harbor pull credential file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("gitops: Harbor pull credential file must be an owner-only regular file")
	}
	data, err := os.ReadFile(d.opts.DockerConfigFile)
	if err != nil {
		return fmt.Errorf("gitops: read Harbor pull credential file: %w", err)
	}
	var config struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("gitops: invalid Docker config JSON")
	}
	if len(config.Auths[d.opts.RegistryHost]) == 0 {
		return fmt.Errorf("gitops: Docker config has no configured Harbor registry")
	}
	object := map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": d.opts.PullSecretName, "namespace": namespace}, "type": "kubernetes.io/dockerconfigjson", "stringData": map[string]any{".dockerconfigjson": string(data)}}
	if err := d.cli().Apply(ctx, []map[string]any{object}); err != nil {
		return fmt.Errorf("gitops: install Harbor pull secret: %w", err)
	}
	return nil
}

func (d *Deployer) WaitReady(ctx context.Context, target execution.Target, refs []execution.WorkloadRef) error {
	if len(refs) != 1 || refs[0].Kind != "Deployment" {
		return fmt.Errorf("gitops: expected one Deployment readiness reference")
	}
	path, err := d.scope(target, refs[0].Name)
	if err != nil {
		return err
	}
	deploymentID := target.Extra["deployment"]
	if deploymentID == "" {
		return fmt.Errorf("gitops: target deployment revision is missing")
	}
	d.mu.Lock()
	rev, ok := d.last[path+"/"+deploymentID]
	d.mu.Unlock()
	if !ok {
		return fmt.Errorf("gitops: no pushed revision for workload")
	}
	defer func() {
		d.mu.Lock()
		delete(d.last, path+"/"+deploymentID)
		d.mu.Unlock()
	}()
	if err := d.waitCommit(ctx, rev.commit); err != nil {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, d.opts.Timeout)
	defer cancel()
	for {
		object, exists, err := d.cli().Get(deadline, target.Namespace, "deployment", refs[0].Name)
		if err == nil && exists && deploymentLabel(object) == rev.deploymentID {
			return d.cli().RolloutStatus(deadline, target.Namespace, "deployment", refs[0].Name, d.opts.Timeout)
		}
		if err := pause(deadline); err != nil {
			return fmt.Errorf("gitops: Fleet did not apply the expected workload revision: %w", err)
		}
	}
}

func deploymentLabel(object map[string]any) string {
	metadata, _ := object["metadata"].(map[string]any)
	labels, _ := metadata["labels"].(map[string]any)
	value, _ := labels["orchestrator.io/deployment-id"].(string)
	return value
}

func (d *Deployer) Remove(ctx context.Context, target execution.Target, workloadID string) error {
	path, err := d.scope(target, workloadID)
	if err != nil {
		return err
	}
	d.mu.Lock()
	commit, err := d.writeAndPush(ctx, path, target.Namespace, nil, true)
	d.mu.Unlock()
	if err != nil {
		return err
	}
	if err := d.waitCommit(ctx, commit); err != nil {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, d.opts.Timeout)
	defer cancel()
	for {
		_, deploymentExists, deploymentErr := d.cli().Get(deadline, target.Namespace, "deployment", workloadID)
		_, serviceExists, serviceErr := d.cli().Get(deadline, target.Namespace, "service", workloadID)
		if deploymentErr == nil && serviceErr == nil && !deploymentExists && !serviceExists {
			// Resource-output Secrets are installed out of band, never in Git.
			return d.cli().Delete(deadline, target.Namespace, "secret", workloadID+"-env")
		}
		if err := pause(deadline); err != nil {
			return fmt.Errorf("gitops: Fleet did not remove workload: %w", err)
		}
	}
}

func (d *Deployer) waitCommit(ctx context.Context, expected string) error {
	deadline, cancel := context.WithTimeout(ctx, d.opts.Timeout)
	defer cancel()
	for {
		object, exists, err := d.cli().Get(deadline, "fleet-local", "gitrepo", d.opts.GitRepoName)
		if err == nil && exists {
			status, _ := object["status"].(map[string]any)
			observed, _ := status["commit"].(string)
			if observed == expected || (observed != "" && d.ancestor(deadline, expected, observed)) {
				return nil
			}
		}
		if err := pause(deadline); err != nil {
			return fmt.Errorf("gitops: Fleet did not observe Git revision %s: %w", expected, err)
		}
	}
}

func (d *Deployer) ancestor(ctx context.Context, older, newer string) bool {
	_, err := d.git(ctx, "merge-base", "--is-ancestor", older, newer)
	return err == nil
}

func pause(ctx context.Context) error {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (d *Deployer) writeAndPush(ctx context.Context, relative, namespace string, manifests []execution.Manifest, remove bool) (string, error) {
	if !remove {
		for _, manifest := range manifests {
			if manifest.Secret || manifest.Kind == "Secret" {
				return "", fmt.Errorf("gitops: refusing to commit a Secret")
			}
			if !segment.MatchString(strings.ToLower(manifest.Kind)) || !segment.MatchString(manifest.Name) {
				return "", fmt.Errorf("gitops: invalid manifest identity")
			}
		}
	}
	status, err := d.git(ctx, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(status) != "" {
		return "", fmt.Errorf("gitops: GitOps worktree has uncommitted changes")
	}
	if _, err := d.git(ctx, "pull", "--ff-only", "origin", d.opts.Branch); err != nil {
		return "", err
	}
	dir := filepath.Join(d.opts.RepoDir, relative)
	if remove {
		entries, err := os.ReadDir(dir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				return "", fmt.Errorf("gitops: bundle contains an unexpected directory")
			}
			if entry.Name() != "fleet.yaml" && !strings.HasSuffix(entry.Name(), ".json") {
				return "", fmt.Errorf("gitops: bundle contains an unexpected file")
			}
		}
		for _, entry := range entries {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return "", err
			}
		}
		if len(entries) > 0 {
			if err := os.Remove(dir); err != nil {
				return "", err
			}
		}
	} else {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if entry.IsDir() || (entry.Name() != "fleet.yaml" && !strings.HasSuffix(entry.Name(), ".json")) {
				return "", fmt.Errorf("gitops: bundle contains an unexpected file")
			}
		}
		for _, entry := range entries {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return "", err
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "fleet.yaml"), []byte("namespace: "+namespace+"\n"), 0o644); err != nil {
			return "", err
		}
		sort.Slice(manifests, func(i, j int) bool { return manifests[i].Kind+manifests[i].Name < manifests[j].Kind+manifests[j].Name })
		for _, manifest := range manifests {
			payload, err := json.MarshalIndent(manifest.Object, "", "  ")
			if err != nil {
				return "", err
			}
			name := strings.ToLower(manifest.Kind) + "-" + manifest.Name + ".json"
			if err := os.WriteFile(filepath.Join(dir, name), append(payload, '\n'), 0o644); err != nil {
				return "", err
			}
		}
	}
	if _, err := d.git(ctx, "add", "-A", "--", relative); err != nil {
		return "", err
	}
	if _, err := d.git(ctx, "diff", "--cached", "--quiet"); err != nil {
		if _, err := d.git(ctx, "commit", "-m", "Deploy "+relative); err != nil {
			return "", err
		}
	}
	commit, err := d.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if _, err := d.git(ctx, "push", "origin", "HEAD:"+d.opts.Branch); err != nil {
		return "", err
	}
	return strings.TrimSpace(commit), nil
}

func (d *Deployer) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", d.opts.RepoDir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gitops: git %s failed: %w: %s", args[0], err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

var _ execution.WorkloadDeployer = (*Deployer)(nil)
