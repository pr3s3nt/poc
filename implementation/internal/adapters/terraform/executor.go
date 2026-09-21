// Package terraform implements the Terraform-backed ResourceExecutor used by the
// aws-eks Execution Profile. Each resource node gets its own working directory
// and its own local state file under the run directory, so one verification run
// never shares state with another.
package terraform

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"orchestrator/internal/ports/execution"
)

//go:embed modules
var moduleFS embed.FS

// Module names supported by the executor.
const (
	ModuleVPC    = "vpc"
	ModuleEKS    = "eks"
	ModuleAurora = "aurora"
)

// Executor runs Terraform for one resource node at a time.
type Executor struct {
	BinaryPath  string
	AWSPath     string
	Root        string
	Region      string
	Tags        map[string]string
	PluginCache string
	Timeout     time.Duration
}

// Options configures the Terraform executor.
type Options struct {
	BinaryPath  string
	AWSPath     string
	Root        string
	Region      string
	Tags        map[string]string
	PluginCache string
}

// New returns a Terraform executor writing state under root.
func New(opts Options) (*Executor, error) {
	if opts.Root == "" {
		return nil, fmt.Errorf("terraform: a run root directory is required")
	}
	if opts.Region == "" {
		return nil, fmt.Errorf("terraform: an AWS region is required")
	}
	if err := os.MkdirAll(opts.Root, 0o700); err != nil {
		return nil, fmt.Errorf("terraform: create run root: %w", err)
	}
	binary := opts.BinaryPath
	if binary == "" {
		binary = "terraform"
	}
	awsPath := opts.AWSPath
	if awsPath == "" {
		awsPath = "aws"
	}
	cache := opts.PluginCache
	if cache != "" {
		if err := os.MkdirAll(cache, 0o700); err != nil {
			return nil, fmt.Errorf("terraform: create plugin cache: %w", err)
		}
	}
	return &Executor{
		BinaryPath:  binary,
		AWSPath:     awsPath,
		Root:        opts.Root,
		Region:      opts.Region,
		Tags:        opts.Tags,
		PluginCache: cache,
		Timeout:     45 * time.Minute,
	}, nil
}

// Provision implements execution.ResourceExecutor for the terraform driver.
func (e *Executor) Provision(ctx context.Context, req execution.ProvisionRequest) (execution.ProvisionResult, error) {
	module := req.Module
	if module == "" {
		return execution.ProvisionResult{}, fmt.Errorf("terraform: resource %s has no module in its definition source", req.Descriptor)
	}
	dir := filepath.Join(e.Root, workspaceName(req.Descriptor))
	if err := e.writeModule(module, dir); err != nil {
		return execution.ProvisionResult{}, err
	}

	vars, err := e.buildVars(module, dir, req)
	if err != nil {
		return execution.ProvisionResult{}, err
	}
	if err := writeVars(dir, vars); err != nil {
		return execution.ProvisionResult{}, err
	}
	if err := e.recordOrder(module, dir); err != nil {
		return execution.ProvisionResult{}, err
	}

	if err := e.run(ctx, dir, "init", "-input=false", "-no-color"); err != nil {
		return execution.ProvisionResult{}, err
	}
	applyErr := e.planAndApply(ctx, dir, module)
	if applyErr != nil && module == ModuleEKS && vars["node_capacity_type"] == "SPOT" && isCapacityError(applyErr) {
		// Spot capacity is best effort; fall back to the smallest on-demand node.
		vars["node_capacity_type"] = "ON_DEMAND"
		if err := writeVars(dir, vars); err != nil {
			return execution.ProvisionResult{}, err
		}
		applyErr = e.planAndApply(ctx, dir, module)
	}
	if applyErr != nil {
		return execution.ProvisionResult{}, applyErr
	}

	outputs, err := e.outputs(ctx, dir)
	if err != nil {
		return execution.ProvisionResult{}, err
	}
	return e.mapOutputs(ctx, module, dir, req, vars, outputs)
}

// allowedResourceTypes is the automated plan review: a run may only create the
// resource types the cost policy allows. Anything else, for example a NAT
// Gateway or a load balancer, fails before apply.
var allowedResourceTypes = map[string]map[string]bool{
	ModuleVPC: {
		"aws_vpc": true, "aws_internet_gateway": true, "aws_subnet": true,
		"aws_route_table": true, "aws_route_table_association": true,
	},
	ModuleEKS: {
		"aws_iam_role": true, "aws_iam_role_policy_attachment": true,
		"aws_eks_cluster": true, "aws_eks_node_group": true,
	},
	ModuleAurora: {
		"aws_db_subnet_group": true, "aws_security_group": true,
		"aws_rds_cluster": true, "aws_rds_cluster_instance": true,
	},
}

// maxResourceCount caps the instances of the resources that carry the cost.
var maxResourceCount = map[string]int{
	"aws_eks_cluster": 1, "aws_eks_node_group": 1,
	"aws_rds_cluster": 1, "aws_rds_cluster_instance": 1, "aws_vpc": 1,
}

type planFile struct {
	ResourceChanges []struct {
		Type   string `json:"type"`
		Change struct {
			Actions []string `json:"actions"`
		} `json:"change"`
	} `json:"resource_changes"`
}

// planAndApply writes a plan, reviews it against the cost policy and only then
// applies that exact plan.
func (e *Executor) planAndApply(ctx context.Context, dir, module string) error {
	if err := e.run(ctx, dir, "plan", "-input=false", "-no-color", "-out=tfplan", "-var-file=terraform.tfvars.json"); err != nil {
		return err
	}
	if err := e.reviewPlan(ctx, dir, module); err != nil {
		return err
	}
	return e.run(ctx, dir, "apply", "-input=false", "-no-color", "-auto-approve", "tfplan")
}

func (e *Executor) reviewPlan(ctx context.Context, dir, module string) error {
	cmd := exec.CommandContext(ctx, e.BinaryPath, "show", "-json", "tfplan")
	cmd.Dir = dir
	cmd.Env = e.env()
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("terraform show -json: %w: %s", err, tail(errOut.String(), 400))
	}
	if err := os.WriteFile(filepath.Join(dir, "plan.json"), out.Bytes(), 0o600); err != nil {
		return fmt.Errorf("terraform: write plan review: %w", err)
	}
	var plan planFile
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		return fmt.Errorf("terraform: decode plan: %w", err)
	}
	allowed := allowedResourceTypes[module]
	counts := map[string]int{}
	for _, change := range plan.ResourceChanges {
		creates := false
		for _, action := range change.Change.Actions {
			if action == "create" {
				creates = true
			}
		}
		if !creates {
			continue
		}
		if !allowed[change.Type] {
			return fmt.Errorf("terraform: plan review rejected %q: resource type %q is outside the cost policy of module %q",
				filepath.Base(dir), change.Type, module)
		}
		counts[change.Type]++
	}
	for resourceType, limit := range maxResourceCount {
		if counts[resourceType] > limit {
			return fmt.Errorf("terraform: plan review rejected %q: %d x %s exceeds the limit of %d",
				filepath.Base(dir), counts[resourceType], resourceType, limit)
		}
	}
	return nil
}

func (e *Executor) writeModule(module, dir string) error {
	entries, err := fs.ReadDir(moduleFS, filepath.Join("modules", module))
	if err != nil {
		return fmt.Errorf("terraform: unknown module %q: %w", module, err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("terraform: create workspace: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := moduleFS.ReadFile(filepath.Join("modules", module, entry.Name()))
		if err != nil {
			return fmt.Errorf("terraform: read module file: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, entry.Name()), content, 0o600); err != nil {
			return fmt.Errorf("terraform: write module file: %w", err)
		}
	}
	return nil
}

// buildVars merges the resolved driver variables and resource params with the
// variables the executor owns. Values are passed through untouched, so the
// Terraform module contract checked at planning time is the one applied here.
func (e *Executor) buildVars(module, dir string, req execution.ProvisionRequest) (map[string]any, error) {
	vars := map[string]any{}
	for key, value := range req.Inputs {
		vars[key] = value
	}
	vars["region"] = e.Region
	vars["tags"] = e.Tags
	if _, ok := vars["name"]; !ok {
		vars["name"] = workspaceName(req.Descriptor)
	}
	if module == ModuleAurora {
		password, err := reusePassword(dir)
		if err != nil {
			return nil, err
		}
		vars["master_password"] = password
	}
	return vars, nil
}

// reusePassword keeps the Aurora master password stable across deployments of
// the same run: PostgreSQL only applies it when the cluster is created.
func reusePassword(dir string) (string, error) {
	existing, err := readVars(dir)
	if err == nil {
		if password, ok := existing["master_password"].(string); ok && password != "" {
			return password, nil
		}
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("terraform: generate master password: %w", err)
	}
	return "Aur" + hex.EncodeToString(b[:]), nil
}

func (e *Executor) mapOutputs(
	ctx context.Context,
	module, dir string,
	req execution.ProvisionRequest,
	vars map[string]any,
	outputs map[string]any,
) (execution.ProvisionResult, error) {
	// Module outputs are named after the Resource Type contract, so they pass
	// straight through. Only the cluster kubeconfig is produced by the executor.
	result := execution.ProvisionResult{
		Outputs: map[string]any{},
		State:   map[string]any{"driver": "terraform", "module": module, "workspace": dir},
	}
	for name, value := range outputs {
		result.Outputs[name] = value
	}
	switch module {
	case ModuleVPC:
		result.State["vpcId"] = outputs["id"]
	case ModuleEKS:
		clusterName := fmt.Sprint(outputs["name"])
		kubeconfig := filepath.Join(dir, "kubeconfig")
		if err := e.updateKubeconfig(ctx, clusterName, kubeconfig, clusterName); err != nil {
			return execution.ProvisionResult{}, err
		}
		result.Outputs["kubeContext"] = clusterName
		result.Outputs["kubeconfig"] = kubeconfig
		result.State["clusterName"] = clusterName
		result.State["nodeGroupName"] = outputs["node_group_name"]
		result.State["kubeconfig"] = kubeconfig
		result.Target = &execution.Target{
			Kind: "kubernetes", ClusterName: clusterName, Context: clusterName, Kubeconfig: kubeconfig,
		}
		delete(result.Outputs, "cluster_security_group_id")
		delete(result.Outputs, "node_group_name")
		delete(result.Outputs, "node_role_arn")
	case ModuleAurora:
		if _, ok := result.Outputs["port"].(float64); !ok {
			result.Outputs["port"] = float64(5432)
		}
		result.State["clusterIdentifier"] = outputs["cluster_identifier"]
		result.State["writerIdentifier"] = outputs["writer_identifier"]
		result.State["securityGroupId"] = outputs["security_group_id"]
		delete(result.Outputs, "cluster_identifier")
		delete(result.Outputs, "writer_identifier")
		delete(result.Outputs, "security_group_id")
	}
	_ = vars
	return result, nil
}

func (e *Executor) updateKubeconfig(ctx context.Context, clusterName, path, alias string) error {
	cmd := exec.CommandContext(ctx, e.AWSPath, "eks", "update-kubeconfig",
		"--region", e.Region, "--name", clusterName, "--kubeconfig", path, "--alias", alias)
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("terraform: aws eks update-kubeconfig: %w: %s", err, strings.TrimSpace(errOut.String()))
	}
	return nil
}

func (e *Executor) run(ctx context.Context, dir string, args ...string) error {
	runCtx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, e.BinaryPath, args...)
	cmd.Dir = dir
	cmd.Env = e.env()
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	logPath := filepath.Join(dir, "terraform.log")
	entry := fmt.Sprintf("\n=== %s %s\n%s\n%s\n", time.Now().UTC().Format(time.RFC3339), strings.Join(args, " "), out.String(), errOut.String())
	if file, ferr := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); ferr == nil {
		_, _ = file.WriteString(entry)
		_ = file.Close()
	}
	if err != nil {
		return fmt.Errorf("terraform %s in %s: %w: %s", strings.Join(args, " "), filepath.Base(dir), err, tail(errOut.String(), 1200))
	}
	return nil
}

func (e *Executor) outputs(ctx context.Context, dir string) (map[string]any, error) {
	cmd := exec.CommandContext(ctx, e.BinaryPath, "output", "-json")
	cmd.Dir = dir
	cmd.Env = e.env()
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("terraform output in %s: %w: %s", filepath.Base(dir), err, tail(errOut.String(), 400))
	}
	raw := map[string]struct {
		Value any `json:"value"`
	}{}
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		return nil, fmt.Errorf("terraform: decode outputs: %w", err)
	}
	result := map[string]any{}
	for key, value := range raw {
		result[key] = value.Value
	}
	return result, nil
}

func (e *Executor) env() []string {
	env := append(os.Environ(),
		"TF_IN_AUTOMATION=1",
		"TF_INPUT=0",
		"AWS_REGION="+e.Region,
		"AWS_DEFAULT_REGION="+e.Region,
	)
	if e.PluginCache != "" {
		env = append(env, "TF_PLUGIN_CACHE_DIR="+e.PluginCache)
	}
	return env
}

// recordOrder appends the workspace to the run manifest so cleanup can destroy
// resources in reverse creation order even if the process stops early.
func (e *Executor) recordOrder(module, dir string) error {
	manifest := filepath.Join(e.Root, "workspaces.txt")
	existing, _ := os.ReadFile(manifest)
	line := module + " " + dir
	for _, l := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(l) == line {
			return nil
		}
	}
	file, err := os.OpenFile(manifest, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("terraform: record workspace: %w", err)
	}
	defer file.Close()
	_, err = file.WriteString(line + "\n")
	return err
}

func writeVars(dir string, vars map[string]any) error {
	payload, err := json.MarshalIndent(vars, "", "  ")
	if err != nil {
		return fmt.Errorf("terraform: encode variables: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "terraform.tfvars.json"), payload, 0o600)
}

func readVars(dir string) (map[string]any, error) {
	payload, err := os.ReadFile(filepath.Join(dir, "terraform.tfvars.json"))
	if err != nil {
		return nil, err
	}
	vars := map[string]any{}
	if err := json.Unmarshal(payload, &vars); err != nil {
		return nil, err
	}
	return vars, nil
}

func workspaceName(descriptor string) string {
	name := strings.NewReplacer(".", "-", "#", "-", "_", "-").Replace(descriptor)
	return strings.ToLower(strings.Trim(name, "-"))
}

func isCapacityError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{"insufficientinstancecapacity", "unfulfillable capacity", "spot", "capacity is not available"} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

var _ execution.ResourceExecutor = (*Executor)(nil)
