// Package providers resolves the pinned provider bindings of a Terraform
// invocation into a private subprocess environment: the Environment AWS
// account and region, a scoped Kubernetes Connection or the completed opaque
// EKS target. It never reads the host credential chain, profile, shared
// config, instance metadata or the default kube context, and it never writes
// a credential anywhere but an owner-only file under a private directory that
// Close removes.
package providers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/terraformexec"
)

// Binder implements terraformexec.ProviderBinder.
type Binder struct {
	AWS  terraformexec.AWSIdentitySource
	Kube terraformexec.KubernetesIdentitySource
	API  terraformexec.AWSAPI
	// Dir is the parent of the private per-invocation directories; empty uses
	// the system temporary directory.
	Dir string
}

var _ terraformexec.ProviderBinder = (*Binder)(nil)

func refuse(why string) error {
	return fmt.Errorf("%w: %s", terraformexec.ErrProviderBinding, why)
}

// Bind implements terraformexec.ProviderBinder.
func (b *Binder) Bind(ctx context.Context, req terraformexec.BindRequest) (terraformexec.ProviderEnv, error) {
	if b.AWS == nil || b.Kube == nil || b.API == nil || req.OrganizationKey == "" {
		return terraformexec.ProviderEnv{}, refuse("the binder is not configured")
	}
	dir, err := os.MkdirTemp(b.Dir, "provider-")
	if err != nil {
		return terraformexec.ProviderEnv{}, refuse("a private directory could not be created")
	}
	closer := func() error { return os.RemoveAll(dir) }
	out := terraformexec.ProviderEnv{Close: closer, Retain: func(handle string) error {
		if !strings.HasPrefix(handle, "retained-") || strings.ContainsAny(handle, "/\\ ") {
			return refuse("invalid retained handle")
		}
		return os.WriteFile(filepath.Join(filepath.Dir(dir), ".provider-"+handle), []byte(filepath.Base(dir)), 0o600)
	}}
	fail := func(err error) (terraformexec.ProviderEnv, error) {
		_ = closer()
		return terraformexec.ProviderEnv{}, err
	}
	pins := append([]tfmodule.ProviderPin(nil), req.Pins...)
	sort.Slice(pins, func(i, j int) bool { return pins[i].Name < pins[j].Name })
	var awsVerified *terraformexec.AWSConnection
	seenAWS, seenKube := false, false
	verify := func(conn terraformexec.AWSConnection) error {
		if awsVerified != nil && awsVerified.ConnectionKey == conn.ConnectionKey {
			return nil
		}
		if conn.AccountID == "" {
			return refuse("the AWS Connection has no pinned account")
		}
		account, err := b.API.CallerAccount(ctx, conn)
		if err != nil || account != conn.AccountID {
			return refuse("the AWS credential does not belong to the Connection account")
		}
		awsVerified = &conn
		return nil
	}
	for _, pin := range pins {
		switch pin.Kind {
		case tfmodule.BindingAWS:
			if seenAWS || pin.ConnectionKey == "" || pin.ConnectionKind != "AWS" {
				return fail(refuse("the AWS binding is incomplete"))
			}
			seenAWS = true
			conn, err := b.AWS.ResolveAWS(ctx, req.OrganizationKey, pin.ConnectionKey)
			if err != nil || !conn.AllowsRegion(pin.Region) || conn.ConnectionKey != pin.ConnectionKey {
				return fail(refuse("the AWS Connection is unavailable or no longer matches the plan"))
			}
			// The Environment pinned one of the verified allowed regions.
			conn.Region = pin.Region
			if err := verify(conn); err != nil {
				return fail(err)
			}
			out.AccountID, out.Region = conn.AccountID, conn.Region
			out.Env = append(out.Env, awsEnv(dir, conn)...)
			if err := writeEmptyAWSConfig(dir); err != nil {
				return fail(err)
			}
		case tfmodule.BindingKubernetes:
			if seenKube {
				return fail(refuse("only one Kubernetes provider binding is supported per invocation"))
			}
			seenKube = true
			if pin.ConnectionKey != "" {
				if pin.ConnectionKind != "KUBERNETES" {
					return fail(refuse("the Kubernetes binding is incomplete"))
				}
				kubeconfig, kubeContext, err := b.Kube.ResolveKubernetes(ctx, req.OrganizationKey, pin.ConnectionKey)
				if err != nil || kubeContext == "" || len(kubeconfig) == 0 {
					return fail(refuse("the Kubernetes Connection is unavailable"))
				}
				path := filepath.Join(dir, "kubeconfig")
				if err := os.WriteFile(path, kubeconfig, 0o600); err != nil {
					return fail(refuse("the kubeconfig could not be written privately"))
				}
				out.Env = append(out.Env, "KUBE_CONFIG_PATH="+path, "KUBE_CTX="+kubeContext)
				continue
			}
			target, ok := req.Targets[pin.Provider]
			if !ok || target.Provider != "eks" || target.Organization != req.OrganizationKey || target.Connection == "" || target.Region == "" || target.ClusterName == "" {
				return fail(refuse("the cluster target is missing, foreign or not a completed EKS target"))
			}
			conn, err := b.AWS.ResolveAWS(ctx, req.OrganizationKey, target.Connection)
			if err != nil || !conn.AllowsRegion(target.Region) || conn.ConnectionKey != target.Connection {
				return fail(refuse("the EKS target's AWS Connection is unavailable or changed"))
			}
			conn.Region = target.Region
			if err := verify(conn); err != nil {
				return fail(err)
			}
			endpoint, err := b.API.DescribeCluster(ctx, conn, target.ClusterName)
			if err != nil || endpoint.Status != "ACTIVE" {
				return fail(refuse("the EKS cluster is not reachable and active"))
			}
			token, err := b.API.ClusterToken(conn, target.ClusterName)
			if err != nil {
				return fail(refuse("the EKS token could not be derived"))
			}
			if out.AccountID == "" {
				out.AccountID, out.Region = conn.AccountID, conn.Region
			}
			out.Env = append(out.Env, "KUBE_HOST="+endpoint.Endpoint, "KUBE_CLUSTER_CA_CERT_DATA="+endpoint.CAPEM, "KUBE_TOKEN="+token, "KUBE_LOAD_CONFIG_FILE=false")
		default:
			return fail(refuse("unknown provider binding kind"))
		}
	}
	if len(pins) == 0 {
		return fail(errors.Join(refuse("a Terraform module needs at least one provider binding")))
	}
	return out, nil
}

// awsEnv is the complete AWS provider environment of one explicit key: no
// shared credentials or config file, no profile and no instance metadata.
func awsEnv(dir string, c terraformexec.AWSConnection) []string {
	env := []string{
		"AWS_ACCESS_KEY_ID=" + c.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + c.SecretAccessKey,
		"AWS_REGION=" + c.Region,
		"AWS_DEFAULT_REGION=" + c.Region,
		"AWS_SHARED_CREDENTIALS_FILE=" + filepath.Join(dir, "aws-credentials"),
		"AWS_CONFIG_FILE=" + filepath.Join(dir, "aws-config"),
		"AWS_EC2_METADATA_DISABLED=true",
		"AWS_STS_REGIONAL_ENDPOINTS=regional",
	}
	if c.SessionToken != "" {
		env = append(env, "AWS_SESSION_TOKEN="+c.SessionToken)
	}
	return env
}

func writeEmptyAWSConfig(dir string) error {
	for _, name := range []string{"aws-credentials", "aws-config"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			return refuse("a private AWS config could not be written")
		}
	}
	return nil
}
