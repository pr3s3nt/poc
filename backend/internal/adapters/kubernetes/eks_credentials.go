package kubernetes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/terraformexec"
)

// TargetCredentials resolves either an uploaded KUBECONFIG Connection or the
// short-lived credential material for a completed, scoped EKS target. EKS
// credentials are derived on each operation and only materialized by OpenCLI
// in its private per-operation directory.
type TargetCredentials struct {
	Kubernetes execution.KubeconfigSource
	AWS        terraformexec.AWSIdentitySource
	API        terraformexec.AWSAPI
}

var _ execution.KubeconfigSource = (*TargetCredentials)(nil)

type kubeconfigDocument struct {
	APIVersion     string              `json:"apiVersion"`
	Kind           string              `json:"kind"`
	Clusters       []kubeconfigCluster `json:"clusters"`
	Contexts       []kubeconfigContext `json:"contexts"`
	CurrentContext string              `json:"current-context"`
	Users          []kubeconfigUser    `json:"users"`
}

type kubeconfigCluster struct {
	Name    string `json:"name"`
	Cluster struct {
		Server                   string `json:"server"`
		CertificateAuthorityData string `json:"certificate-authority-data"`
	} `json:"cluster"`
}

type kubeconfigContext struct {
	Name    string `json:"name"`
	Context struct {
		Cluster string `json:"cluster"`
		User    string `json:"user"`
	} `json:"context"`
}

type kubeconfigUser struct {
	Name string `json:"name"`
	User struct {
		Token string `json:"token"`
	} `json:"user"`
}

// ResolveKubeconfig chooses only an explicitly named target source. Unknown or
// incomplete provider metadata never falls back to the Environment's
// KUBECONFIG credential.
func (s *TargetCredentials) ResolveKubeconfig(ctx context.Context, target execution.Target) ([]byte, error) {
	if target.Provider == "" {
		if target.Region != "" || s.Kubernetes == nil {
			return nil, ErrCredentialTarget
		}
		return s.Kubernetes.ResolveKubeconfig(ctx, target)
	}
	if target.Provider != "eks" || target.Kind != "kubernetes" || target.Organization == "" || target.Connection == "" ||
		target.Region == "" || target.ClusterName == "" || (target.Context != "" && target.Context != target.ClusterName) ||
		target.Kubeconfig != "" || s.AWS == nil || s.API == nil {
		return nil, ErrCredentialTarget
	}
	conn, err := s.AWS.ResolveAWS(ctx, target.Organization, target.Connection)
	if err != nil || conn.ConnectionKey != target.Connection {
		return nil, ErrCredentialTarget
	}
	conn.Region = target.Region
	account, err := s.API.CallerAccount(ctx, conn)
	if err != nil || account == "" || account != conn.AccountID {
		return nil, ErrCredentialTarget
	}
	cluster, err := s.API.DescribeCluster(ctx, conn, target.ClusterName)
	if err != nil || cluster.Status != "ACTIVE" {
		return nil, ErrCredentialTarget

	}
	endpoint, err := url.Parse(cluster.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || !strings.Contains(cluster.CAPEM, "BEGIN CERTIFICATE") {
		return nil, ErrCredentialTarget
	}
	token, err := s.API.ClusterToken(conn, target.ClusterName)
	if err != nil || token == "" {
		return nil, ErrCredentialTarget
	}
	return encodeEKSKubeconfig(target.ClusterName, cluster.Endpoint, cluster.CAPEM, token)
}

func encodeEKSKubeconfig(name, endpoint, caPEM, token string) ([]byte, error) {
	if name == "" || endpoint == "" || caPEM == "" || token == "" {
		return nil, ErrCredentialTarget
	}
	doc := kubeconfigDocument{APIVersion: "v1", Kind: "Config", CurrentContext: name}
	cluster := kubeconfigCluster{Name: name}
	cluster.Cluster.Server = endpoint
	cluster.Cluster.CertificateAuthorityData = base64.StdEncoding.EncodeToString([]byte(caPEM))
	doc.Clusters = []kubeconfigCluster{cluster}
	context := kubeconfigContext{Name: name}
	context.Context.Cluster, context.Context.User = name, name
	doc.Contexts = []kubeconfigContext{context}
	user := kubeconfigUser{Name: name}
	user.User.Token = token
	doc.Users = []kubeconfigUser{user}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, errors.New("kubernetes: EKS credentials could not be encoded")
	}
	return encoded, nil
}
