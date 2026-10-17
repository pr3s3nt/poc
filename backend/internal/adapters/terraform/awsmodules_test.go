package terraform_test

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/moduleregistry"
)

// Versions verified on 2026-10-13: the AWS provider 6.60.0 (published
// 2026-08-13); EKS 1.35 (standard support to 2027-03-27); Aurora PostgreSQL
// 16.14 (describe-db-engine-versions, us-east-1, Serverless v2 minimum 0 ACU);
// ElastiCache for Redis OSS 7.1 (latest Redis OSS engine of the service).
const (
	awsProviderVersion = "6.60.0"
	eksVersion         = "1.35"
)

var awsModules = []string{"vpc", "eks", "aurora", "elasticache"}

func awsBundles(t *testing.T) map[string]tfmodule.Bundle {
	t.Helper()
	svc := registrytest.Open(t, store.New())
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]tfmodule.Bundle{}
	for _, id := range awsModules {
		bundle, err := svc.Bundle(t.Context(), refs[id])
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		out[id] = bundle
	}
	return out
}

// The pinned provider is the verified release and every AWS module admits it
// only through a bounded range, so no module can float to another major line.
func TestAWSProviderPinAndModuleConstraints(t *testing.T) {
	release, _ := registry.DefaultRelease()
	var aws registry.ReleaseProvider
	for _, p := range release.Providers {
		if strings.HasSuffix(p.Source, "/hashicorp/aws") {
			aws = p
		}
	}
	h1 := 0
	for _, h := range aws.Hashes {
		if strings.HasPrefix(h, "h1:") {
			h1++
		}
	}
	if aws.Version != awsProviderVersion || aws.Binding != tfmodule.BindingAWS || h1 < 4 || len(aws.Hashes) < 8 {
		t.Fatalf("aws provider pin %+v (h1 hashes %d)", aws, h1)
	}
	if release.Terraform.Version != "1.9.8" {
		t.Fatalf("terraform release %s", release.Terraform.Version)
	}
	for id, b := range awsBundles(t) {
		p := b.Providers["aws"]
		if p.Version != awsProviderVersion || p.Binding != tfmodule.BindingAWS || p.Constraints != ">= 6.60.0, < 7.0.0" {
			t.Errorf("%s provider %+v", id, p)
		}
		if len(b.Providers) != 1 || len(b.Contract.Transitive) != 0 || b.Contract.SourceKind != tfmodule.SourceKindPackaged {
			t.Errorf("%s providers %v transitive %v kind %s", id, b.Providers, b.Contract.Transitive, b.Contract.SourceKind)
		}
	}
}

// Registry schemas: inputs, executor-owned names, outputs and the Resource
// Types each AWS module serves.
func TestAWSModuleRegistryContracts(t *testing.T) {
	all := awsBundles(t)
	type want struct {
		types, owned, executorOutputs []string
		inputs                        map[string]string
		required                      []string
		outputs                       map[string]string
		sensitive                     []string
	}
	wants := map[string]want{
		"vpc": {types: []string{"vpc"}, owned: []string{"tags"},
			inputs:   map[string]string{"region": "string", "name": "string", "cidr": "string", "az_count": "number", "tags": "map(string)"},
			required: []string{"region", "name"},
			outputs:  map[string]string{"id": "", "cidr": "", "subnetIds": "", "privateSubnetIds": ""}},
		"eks": {types: []string{"k8s-cluster"}, owned: []string{"tags"}, executorOutputs: []string{"kubeContext", "kubeconfig"},
			inputs: map[string]string{"region": "string", "name": "string", "subnet_ids": "list(string)", "kubernetes_version": "string", "node_instance_type": "string",
				"node_capacity_type": "string", "node_disk_size": "number", "node_ami_type": "string", "ebs_csi_driver": "bool", "tags": "map(string)", "public_access_cidrs": "list(string)"},
			required: []string{"region", "name", "subnet_ids", "kubernetes_version", "node_instance_type", "node_capacity_type", "node_disk_size", "node_ami_type", "public_access_cidrs"},
			outputs:  map[string]string{"name": "", "endpoint": "", "cluster_security_group_id": "", "node_group_name": "", "node_role_arn": ""}},
		"aurora": {types: []string{"postgres"}, owned: []string{"master_password", "tags"},
			inputs: map[string]string{"region": "string", "name": "string", "vpc_id": "string", "private_subnet_ids": "list(string)", "subnet_ids": "list(string)", "database": "string", "username": "string",
				"master_password": "string", "tags": "map(string)", "engine_version": "string", "min_capacity": "number", "max_capacity": "number", "seconds_until_auto_pause": "number"},
			required: []string{"region", "name", "vpc_id", "private_subnet_ids", "subnet_ids", "database", "username", "master_password", "tags", "engine_version", "min_capacity", "max_capacity"},
			outputs: map[string]string{"host": "", "port": "", "database": "", "username": "", "password": "", "cluster_identifier": "", "writer_identifier": "",
				"security_group_id": ""}, sensitive: []string{"password"}},
		"elasticache": {types: []string{"redis"}, owned: []string{"password", "tags"},
			inputs: map[string]string{"region": "string", "vpc_id": "string", "private_subnet_ids": "list(string)", "subnet_ids": "list(string)", "username": "string", "engine_version": "string",
				"node_type": "string", "password": "string", "tags": "map(string)"},
			required:  []string{"region", "vpc_id", "private_subnet_ids", "subnet_ids", "password", "tags", "engine_version", "node_type"},
			outputs:   map[string]string{"host": "", "port": "", "username": "", "password": "", "tls": "", "replication_group_id": "", "security_group_id": ""},
			sensitive: []string{"password"}},
	}
	for id, w := range wants {
		b := all[id]
		if !equal(b.Contract.ResourceTypes, w.types) || !equal(b.Contract.ExecutorOwned, w.owned) || !equal(b.Contract.ExecutorOutputs, w.executorOutputs) {
			t.Errorf("%s types %v owned %v executor outputs %v", id, b.Contract.ResourceTypes, b.Contract.ExecutorOwned, b.Contract.ExecutorOutputs)
		}
		if len(b.Contract.Inputs) != len(w.inputs) {
			t.Errorf("%s inputs %v", id, b.Contract.Inputs)
		}
		for name, typ := range w.inputs {
			if got := b.Contract.Inputs[name].Type; got != typ {
				t.Errorf("%s input %s type %q, want %q", id, name, got, typ)
			}
		}
		var required []string
		for name, in := range b.Contract.Inputs {
			if in.Required {
				required = append(required, name)
			}
		}
		if !equal(required, w.required) {
			t.Errorf("%s required %v, want %v", id, required, w.required)
		}
		if len(b.Contract.Outputs) != len(w.outputs) {
			t.Errorf("%s outputs %v", id, b.Contract.Outputs)
		}
		for name := range w.outputs {
			out, ok := b.Contract.Outputs[name]
			if !ok || out.Sensitive != contains(w.sensitive, name) {
				t.Errorf("%s output %s = %+v, want sensitive %v", id, name, out, contains(w.sensitive, name))
			}
		}
		// Credentials are executor-owned, sensitive and never defaulted.
		for name, in := range b.Contract.Inputs {
			if (name == "password" || name == "master_password" || in.Sensitive) && (in.HasDefault || !in.Sensitive) {
				t.Errorf("%s secret input %s must be sensitive without a default", id, name)
			}
		}
		// Account, region and credentials are never module inputs: only the
		// executor-pinned region exists and nothing selects an account.
		for name := range b.Contract.Inputs {
			if strings.Contains(name, "account") || strings.Contains(name, "profile") || strings.Contains(name, "access_key") || strings.Contains(name, "assume_role") || strings.Contains(name, "endpoint") {
				t.Errorf("%s exposes provider/account selection input %s", id, name)
			}
		}
	}
	// Engine versions and sizing are supplied by the Definition (no invented
	// module default); only the auto-pause interval keeps its default.
	for _, c := range []struct{ module, input string }{{"aurora", "engine_version"}, {"aurora", "min_capacity"}, {"aurora", "max_capacity"}, {"elasticache", "engine_version"}, {"elasticache", "node_type"}} {
		if all[c.module].Contract.Inputs[c.input].HasDefault {
			t.Errorf("%s %s must not have a module default", c.module, c.input)
		}
	}
	if d := all["aurora"].Contract.Inputs["seconds_until_auto_pause"].Default; d != float64(3600) {
		t.Fatalf("auto-pause default %v", d)
	}
}

// Output mappings: every Resource Type output is covered by a compatible
// module output with the same secret classification.
func TestAWSModuleOutputMappings(t *testing.T) {
	all := awsBundles(t)
	cases := []struct {
		module string
		typ    resource.Type
		// mapping of Type output -> module output (kubeContext is added by the executor).
		skip   []string
		values map[string]any
	}{
		{"vpc", seedType(t, "vpc"), nil, map[string]any{"id": "vpc-0123", "cidr": "10.42.0.0/16", "subnetIds": []any{"subnet-a", "subnet-b"}}},
		{"eks", seedType(t, "k8s-cluster"), []string{"kubeContext", "kubeconfig"}, map[string]any{"name": "orch-x", "endpoint": "https://x.eks.amazonaws.com", "kubeContext": "orch-x"}},
		{"aurora", seedType(t, "postgres"), nil, map[string]any{"host": "x.cluster-y.us-east-1.rds.amazonaws.com", "port": float64(5432), "database": "acceptance", "username": "app", "password": "p"}},
		{"elasticache", redisType(), nil, map[string]any{"host": "master.x.cache.amazonaws.com", "port": float64(6379), "username": "app", "password": "p", "tls": true}},
	}
	for _, c := range cases {
		b := all[c.module]
		for _, field := range c.typ.Outputs {
			if contains(c.skip, field.Name) {
				continue
			}
			out, ok := b.Contract.Outputs[field.Name]
			if !ok || !tfmodule.OutputCompatible(field.Type, out.Type) {
				t.Errorf("%s output %s (%s) does not satisfy the %s Type", c.module, field.Name, out.Type, c.typ.Key)
			}
			if field.Secret != out.Sensitive {
				t.Errorf("%s output %s sensitive=%v differs from the Type secret=%v", c.module, field.Name, out.Sensitive, field.Secret)
			}
		}
		if err := c.typ.ValidateOutputs(c.values); err != nil {
			t.Errorf("%s sample outputs: %v", c.module, err)
		}
	}
	// The EKS executor outputs are exactly those the executor supplies; the
	// kubeconfig is never produced (the target is an opaque scoped identity).
	eks := all["eks"].Contract
	if !equal(eks.ExecutorOutputs, []string{"kubeContext", "kubeconfig"}) {
		t.Fatalf("eks executor outputs %v", eks.ExecutorOutputs)
	}
	for name := range eks.Outputs {
		if strings.Contains(strings.ToLower(name), "kubeconfig") || strings.Contains(strings.ToLower(name), "token") || strings.Contains(strings.ToLower(name), "certificate") {
			t.Errorf("eks module output %s would publish cluster credentials", name)
		}
	}
	// PostgreSQL params of the Type map to required module variables.
	pg := seedType(t, "postgres")
	for _, in := range pg.Inputs {
		module := all["aurora"].Contract.Inputs[in.Name]
		if !module.Required || !tfmodule.InputCompatible(in.Type, module.Type) {
			t.Errorf("postgres param %s does not map to a required compatible aurora variable", in.Name)
		}
	}
	// Redis has no params: sizing and engine stay in the module/Definition.
	if len(redisType().Inputs) != 0 {
		t.Fatal("the Redis Type must stay parameterless")
	}
}

// codeOnly drops comment lines so policy checks see only configuration.
func codeOnly(src string) string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

var (
	cidrAny    = regexp.MustCompile(`"0\.0\.0\.0/0"|"::/0"`)
	publicTrue = regexp.MustCompile(`(?m)^\s*(publicly_accessible|associate_public_ip_address)\s*=\s*true`)
)

// Static plan policy of the module sources: allowlisted resource types and
// counts, no public database/cache, no NAT/EIP/load balancer/key/endpoint, no
// shared or account-wide object, no executable hook, no credential in metadata.
func TestAWSModuleSourceSafety(t *testing.T) {
	allowed := map[string][]string{
		"vpc": {"aws_default_security_group", "aws_internet_gateway", "aws_route_table", "aws_route_table", "aws_route_table_association", "aws_route_table_association", "aws_subnet", "aws_subnet", "aws_vpc"},
		"eks": {"aws_eks_access_entry", "aws_eks_access_policy_association", "aws_eks_addon", "aws_eks_addon", "aws_eks_cluster", "aws_eks_node_group", "aws_iam_role", "aws_iam_role", "aws_iam_role",
			"aws_iam_role_policy_attachment", "aws_iam_role_policy_attachment", "aws_iam_role_policy_attachment", "aws_iam_role_policy_attachment", "aws_iam_role_policy_attachment"},
		"aurora": {"aws_db_subnet_group", "aws_rds_cluster", "aws_rds_cluster_instance", "aws_security_group"},
		"elasticache": {"aws_elasticache_replication_group", "aws_elasticache_subnet_group", "aws_elasticache_user", "aws_elasticache_user",
			"aws_elasticache_user_group", "aws_security_group"},
	}
	banned := []string{
		"local-exec", "remote-exec", "provisioner", "terraform_remote_state", "external\"", "http\"", "random_", "timestamp()", "uuid()",
		"aws_nat_gateway", "aws_eip", "aws_lb", "aws_alb", "aws_elb", "aws_vpc_endpoint", "aws_kms", "aws_iam_user", "aws_iam_access_key", "aws_iam_policy\"",
		"aws_iam_account", "aws_iam_openid", "aws_s3", "aws_ecr", "aws_cloudwatch", "aws_flow_log", "aws_secretsmanager", "aws_ssm", "aws_instance", "aws_launch_template",
		"aws_security_group_rule", "aws_vpc_peering", "aws_ec2_transit", "aws_default_vpc", "aws_default_subnet", "aws_main_route_table_association",
		"assume_role {", "access_key", "secret_key", "profile", "shared_credentials", "endpoints", "skip_credentials_validation", "prevent_destroy",
		"ignore_changes", "create_before_destroy", "orchestrator-created-by-run", "manage_master_user_password",
	}
	for _, id := range awsModules {
		src := moduleSource(t, id)
		var types []string
		for _, m := range resourceBlock.FindAllStringSubmatch(src, -1) {
			types = append(types, m[1])
		}
		if !equal(types, allowed[id]) {
			t.Errorf("%s resources %v, want %v", id, sorted(types), sorted(allowed[id]))
		}
		for _, b := range banned {
			if strings.Contains(src, b) {
				t.Errorf("%s contains banned %q", id, b)
			}
		}
		if publicTrue.MatchString(src) {
			t.Errorf("%s makes an endpoint public", id)
		}
		// The provider block only receives the pinned region and platform tags.
		if !strings.Contains(src, "region = var.region") || !strings.Contains(src, "tags = var.tags") {
			t.Errorf("%s provider block is not the pinned region and tags", id)
		}
		// Names derive from stable identity (name input or the instance tag).
		if strings.Contains(src, "var.run") || strings.Contains(src, "run_id") {
			t.Errorf("%s derives a name from a run", id)
		}
	}
	// The only 0.0.0.0/0 is the public default route of the VPC node subnets;
	// no security group opens anything to the world.
	for _, id := range awsModules {
		src := moduleSource(t, id)
		n := len(cidrAny.FindAllString(src, -1))
		if (id == "vpc" && n != 1) || (id != "vpc" && n != 0) {
			t.Errorf("%s has %d world-open CIDRs", id, n)
		}
	}
	// Data stores have exactly one ingress rule from the node subnets, no egress
	// and no route in their private route tables.
	for _, id := range []string{"aurora", "elasticache"} {
		src := codeOnly(moduleSource(t, id))
		if strings.Count(src, "ingress {") != 1 || strings.Contains(src, "egress") || strings.Contains(src, "route {") || strings.Contains(src, "gateway_id") {
			t.Errorf("%s network rules are not ingress-from-node-subnets only", id)
		}
		if !strings.Contains(src, "cidr_blocks = local.client_cidrs") {
			t.Errorf("%s must restrict ingress to the node subnets", id)
		}
		// The shared private network belongs to the VPC state: a consumer never
		// creates a subnet, route table, association or CIDR.
		for _, banned := range []string{`resource "aws_subnet"`, `resource "aws_route_table`, "cidrsubnet", "vpc_cidr", "netnum"} {
			if strings.Contains(src, banned) {
				t.Errorf("%s must not own private network objects: %q", id, banned)
			}
		}
		if !strings.Contains(src, "subnet_ids = var.private_subnet_ids") {
			t.Errorf("%s subnet group must use the shared private subnets", id)
		}
		// Credentials appear only in the credential argument and sensitive output.
		for n, line := range strings.Split(src, "\n") {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "#") || !(strings.Contains(trim, "var.password") || strings.Contains(trim, "var.master_password")) {
				continue
			}
			switch trim {
			case "master_password              = var.master_password", "passwords = [var.password]", "value     = var.password", "value     = var.master_password":
			default:
				t.Errorf("%s:%d unexpected password use: %s", id, n+1, trim)
			}
		}
	}
	// The VPC owns the private network once, with no gateway route.
	vpc := codeOnly(moduleSource(t, "vpc"))
	for _, want := range []string{`resource "aws_subnet" "private"`, `resource "aws_route_table" "private"`, `resource "aws_route_table_association" "private"`,
		"cidrsubnet(var.cidr, 4, 8 + count.index)", "map_public_ip_on_launch = false",
		`output "subnetIds" { value = tolist(aws_subnet.public[*].id) }`,
		`output "privateSubnetIds" { value = tolist(aws_subnet.private[*].id) }`} {
		if !strings.Contains(vpc, want) {
			t.Errorf("vpc misses %q", want)
		}
	}
	privateBlock := vpc[strings.Index(vpc, `resource "aws_route_table" "private"`):]
	privateBlock = privateBlock[:strings.Index(privateBlock, "}")+1]
	if strings.Contains(privateBlock, "gateway_id") || strings.Contains(privateBlock, "route {") {
		t.Error("the private route table must have no route")
	}
	// Aurora destroy/privacy policy.
	aurora := moduleSource(t, "aurora")
	for _, want := range []string{"storage_encrypted            = true", "publicly_accessible          = false", "skip_final_snapshot          = true",
		"deletion_protection          = false", "delete_automated_backups     = true", "auto_minor_version_upgrade   = false", "performance_insights_enabled = false", `instance_class               = "db.serverless"`} {
		if !strings.Contains(aurora, want) {
			t.Errorf("aurora misses %q", want)
		}
	}
	// ElastiCache TLS, auth and size policy.
	cache := moduleSource(t, "elasticache")
	for _, want := range []string{"transit_encryption_enabled = true", "at_rest_encryption_enabled = true", "user_group_ids", `type      = "password"`, "num_cache_clusters         = 1",
		"automatic_failover_enabled = false", "snapshot_retention_limit   = 0", "auto_minor_version_upgrade = false"} {
		if !strings.Contains(cache, want) {
			t.Errorf("elasticache misses %q", want)
		}
	}
	// EKS access and standard-support policy.
	eks := moduleSource(t, "eks")
	for _, want := range []string{`authentication_mode                         = "API"`, "bootstrap_cluster_creator_admin_permissions = false", `support_type = "STANDARD"`,
		"AmazonEKSClusterAdminPolicy", "aws_iam_session_context", "enabled_cluster_log_types = []", "public_access_cidrs     = var.public_access_cidrs", "endpoint_private_access = true"} {
		if !strings.Contains(eks, want) {
			t.Errorf("eks misses %q", want)
		}
	}
	// The public API allowlist has no default and is a required list input.
	if i := strings.Index(eks, `variable "public_access_cidrs"`); i < 0 || strings.Contains(eks[i:i+200], "default") {
		t.Error("eks public_access_cidrs must be a required input without a default")
	}
	// IAM: only run-owned roles with AWS-managed policies, never an inline or
	// customer-managed policy.
	if strings.Count(eks, "arn:aws:iam::aws:policy/") != 5 || !strings.Contains(eks, "variable \"ebs_csi_driver\"") || !strings.Contains(eks, "count                       = var.ebs_csi_driver ? 1 : 0") || !strings.Contains(eks, "pods.eks.amazonaws.com") || strings.Contains(eks, "aws_iam_role_policy\"") || strings.Contains(eks, "inline_policy") {
		t.Error("eks IAM is not limited to AWS-managed attachments and the optional Pod Identity role")
	}
}

func TestAWSModulesInitValidateReadonlyLock(t *testing.T) {
	for _, id := range awsModules {
		t.Run(id, func(t *testing.T) {
			dir, bin := prepared(t, id)
			if out, err := tf(t, dir, bin, "validate", "-no-color"); err != nil {
				t.Fatalf("validate: %s", tail(out))
			}
			got, _ := os.ReadFile(filepath.Join(dir, ".terraform.lock.hcl"))
			b := awsBundles(t)[id]
			if string(got) != string(registry.LockFile(b.Providers)) {
				t.Fatal("init changed the pinned lock file")
			}
		})
	}
}

// Plan-policy tests run with the real Terraform CLI and the pinned provider
// schema, but with a mock AWS provider (terraform test mock_provider): no
// credential, no network call and no AWS object. This checks the module logic
// (names, CIDR slots, ingress, encryption, validation bounds), not provider or
// account behavior; live behavior is I04-12.
func TestAWSModulesPlanPolicyWithMockProvider(t *testing.T) {
	for _, id := range awsModules {
		t.Run(id, func(t *testing.T) {
			dir, bin := prepared(t, id)
			test, err := os.ReadFile(filepath.Join("testdata", "awsmodules", id+".tftest.hcl"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, id+".tftest.hcl"), test, 0o600); err != nil {
				t.Fatal(err)
			}
			out, err := tf(t, dir, bin, "test", "-no-color")
			if err != nil || !strings.Contains(string(out), "0 failed") {
				t.Fatalf("terraform test: %s", tail(out))
			}
			if strings.Contains(string(out), "Error:") {
				t.Fatalf("terraform test reported errors: %s", tail(out))
			}
		})
	}
}

// Add-on builds verified on 2026-10-13 (aws eks describe-addon-versions) as
// compatible with every Kubernetes version the eks module accepts.
const (
	ebsCSIBuild       = "v1.66.0-eksbuild.1"
	podIdentityBuild  = "v1.4.0-eksbuild.4"
	supportedK8sRange = `contains(["1.34", "1.35", "1.36", "1.37"], var.kubernetes_version)`
)

func TestEKSAddonsArePinnedToVerifiedBuilds(t *testing.T) {
	src := codeOnly(moduleSource(t, "eks"))
	blocks := strings.Count(src, `resource "aws_eks_addon"`)
	if blocks != 2 || strings.Count(src, "addon_version ") != blocks {
		t.Fatalf("every add-on needs an exact addon_version: %d blocks, %d versions", blocks, strings.Count(src, "addon_version "))
	}
	for _, want := range []string{`ebs_csi_driver_version     = "` + ebsCSIBuild + `"`, `pod_identity_agent_version = "` + podIdentityBuild + `"`,
		"addon_version               = local.pod_identity_agent_version", "addon_version               = local.ebs_csi_driver_version", supportedK8sRange} {
		if !strings.Contains(strings.Join(strings.Fields(src), " "), strings.Join(strings.Fields(want), " ")) {
			t.Errorf("eks misses %q", want)
		}
	}
	// No resolution to a service default or latest, no version input.
	if strings.Contains(src, "most_recent") || strings.Contains(src, "data \"aws_eks_addon_version\"") || strings.Contains(src, `variable "addon_`) {
		t.Error("add-on versions must not be resolved or injected at execution")
	}
	// A Kubernetes version outside the verified range fails closed (tftest
	// extended_support_version_refused covers 1.33 at plan time).
	b := awsBundles(t)["eks"]
	if _, ok := b.Contract.Inputs["kubernetes_version"]; !ok {
		t.Fatal("kubernetes_version input missing")
	}
}

// The release revision is immutable identity: this release is a new revision,
// never a changed digest under an earlier one, and earlier releases (v1 at
// fe63d35, v2 handed to review) are not served by the new manifest.
func TestPackagedReleaseRevisionIsDistinctFromEarlierRelease(t *testing.T) {
	type pin struct{ revision, digest string }
	earlier := map[string][]pin{
		"vpc":           {{"v1-2026.10.13", "ebc9b73046f6838921bc5fcff8beebb725a0d085a8921e95766fb5a30339b92d"}, {"v2-2026.10.13", "7f85d296ce7dceff9d57475599a4367c7ece599cf87d1c5e269b11d0071395e4"}, {"v3-2026.10.13", "f159d1be486706560730a1d7e36744b5588103d7d6ea9c67e1c120519cb382cc"}},
		"eks":           {{"v1-2026.10.13", "00dabd6fe3df5514f1c26bc23064fe527157d27130402a4305abe71503070460"}, {"v2-2026.10.13", "15cd6a4d5a88dc48ff49cce5c84c68db160f934d8594752b933b502952647997"}, {"v3-2026.10.13", "15cd6a4d5a88dc48ff49cce5c84c68db160f934d8594752b933b502952647997"}},
		"aurora":        {{"v1-2026.10.13", "5fc963c62421d46ac1c68540f9fef2651314721facd245967394a63f6293e7f2"}, {"v2-2026.10.13", "b3d4fa946b12a66799818715abbdf64a0ffe12d8a1781e778f8fd86d6ca71afd"}, {"v3-2026.10.13", "f8755b77289c2e2bf5b2be351b90d4a1d44d0eeaf56021560c7fefb1aafc4ba6"}},
		"elasticache":   {{"v2-2026.10.13", "ad562a3f616c6db0566a38d38acd323fc8d62ebeaa68758bb74ea83a556b9878"}, {"v3-2026.10.13", "b46261d722e23f7ace3693c12f0d5ed03f28ca6d8abbbc8ae519230051c3e8e7"}},
		"k8s-namespace": {{"v1-2026.10.13", "836f7b7470deb519633a523dccb02a83a1b20947725ddb705596fb2601a180a8"}, {"v2-2026.10.13", "836f7b7470deb519633a523dccb02a83a1b20947725ddb705596fb2601a180a8"}, {"v3-2026.10.13", "836f7b7470deb519633a523dccb02a83a1b20947725ddb705596fb2601a180a8"}},
		"redis-k8s":     {{"v1-2026.10.13", "feaa167e8a79990aa57e5a6f5f5c5a3de15c2c82312bffe179557e25e1d14e74"}, {"v2-2026.10.13", "feaa167e8a79990aa57e5a6f5f5c5a3de15c2c82312bffe179557e25e1d14e74"}, {"v3-2026.10.13", "feaa167e8a79990aa57e5a6f5f5c5a3de15c2c82312bffe179557e25e1d14e74"}},
		"postgres-k8s":  {{"v1-2026.10.13", "446aeb69eafa37573233ff53629372172b8ce80439e8c8c2440fdbede9a860e1"}, {"v2-2026.10.13", "446aeb69eafa37573233ff53629372172b8ce80439e8c8c2440fdbede9a860e1"}, {"v3-2026.10.13", "446aeb69eafa37573233ff53629372172b8ce80439e8c8c2440fdbede9a860e1"}},
	}
	const current = "v4-2026.10.15"
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	svc := registrytest.Open(t, store.New())
	identities := map[string]string{}
	for id, ref := range refs {
		if ref.Revision != current {
			t.Errorf("%s revision %q", id, ref.Revision)
		}
		key := ref.SourceID + "@" + ref.Revision + "/" + ref.Subdirectory
		if other, dup := identities[key]; dup {
			t.Errorf("%s and %s share the identity %s", id, other, key)
		}
		identities[key] = id
		for _, prev := range earlier[id] {
			if prev.revision == current {
				t.Fatal("an earlier pin cannot use the current revision")
			}
			// Changed content must not keep an earlier pin.
			changed := id == "vpc" || (id == "aurora" && prev.revision != "v3-2026.10.13") || (id == "elasticache" && prev.revision != "v3-2026.10.13") || (id == "eks" && prev.revision == "v1-2026.10.13")
			if changed && ref.ContentSHA256 == prev.digest {
				t.Errorf("%s changed module but kept the digest of %s", id, prev.revision)
			}
			// The earlier pin, exactly as persisted, is not verified by this release
			// and is never silently upgraded to the new one.
			oldRef := ref
			oldRef.Revision, oldRef.ContentSHA256 = prev.revision, prev.digest
			if _, err := svc.Bundle(t.Context(), oldRef); !errors.Is(err, moduleregistry.ErrNotVerified) {
				t.Errorf("%s earlier pin %s: want ErrNotVerified, got %v", id, prev.revision, err)
			}
		}
	}
	if len(refs) != 7 || len(identities) != 7 {
		t.Fatalf("refs %d identities %d", len(refs), len(identities))
	}
}
