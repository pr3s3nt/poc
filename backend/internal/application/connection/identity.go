package connection

import (
	"bytes"
	"context"
	"encoding/json"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/terraformexec"
)

var (
	_ terraformexec.AWSIdentitySource        = (*CredentialResolver)(nil)
	_ terraformexec.KubernetesIdentitySource = (*CredentialResolver)(nil)
)

// awsAccessKeyValue is the stored credential value of an AWS_ACCESS_KEY
// Connection. Unknown fields are refused so no other secret rides along.
type awsAccessKeyValue struct {
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	SessionToken    string `json:"sessionToken,omitempty"`
}

// ResolveAWS implements terraformexec.AWSIdentitySource. It reads the scoped
// access key of one READY AWS_ACCESS_KEY Connection of the Organization and
// returns it with the account and region the Connection pinned. Every
// failure is the same fail-closed error; no host credential is ever used.
func (r *CredentialResolver) ResolveAWS(ctx context.Context, organizationKey, connectionKey string) (terraformexec.AWSConnection, error) {
	if organizationKey == "" || connectionKey == "" || r.connections == nil || r.store == nil {
		return terraformexec.AWSConnection{}, ErrCredentialUnavailable
	}
	conn, err := r.connections.GetConnection(ctx, organizationKey, connectionKey)
	if err != nil || conn.OrganizationKey != organizationKey || conn.Key != connectionKey || conn.Kind != application.ConnectionAWS ||
		conn.AuthenticationType != application.AuthAWSAccessKey || conn.Status != application.ConnectionReady || conn.SecretRef == "" ||
		conn.ConfigString("accountId") == "" || conn.ConfigString("region") == "" {
		return terraformexec.AWSConnection{}, ErrCredentialUnavailable
	}
	raw, err := r.store.Get(ctx, conn.OrganizationKey, conn.Key, conn.SecretRef)
	if err != nil || len(raw) == 0 {
		return terraformexec.AWSConnection{}, ErrCredentialUnavailable
	}
	var value awsAccessKeyValue
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || value.AccessKeyID == "" || value.SecretAccessKey == "" {
		return terraformexec.AWSConnection{}, ErrCredentialUnavailable
	}
	return terraformexec.AWSConnection{
		ConnectionKey: conn.Key, AccessKeyID: value.AccessKeyID, SecretAccessKey: value.SecretAccessKey, SessionToken: value.SessionToken,
		AccountID: conn.ConfigString("accountId"), Region: conn.ConfigString("region"),
	}, nil
}

// ResolveKubernetes implements terraformexec.KubernetesIdentitySource.
func (r *CredentialResolver) ResolveKubernetes(ctx context.Context, organizationKey, connectionKey string) ([]byte, string, error) {
	conn, err := r.Connection(ctx, organizationKey, connectionKey)
	if err != nil || r.store == nil {
		return nil, "", ErrCredentialUnavailable
	}
	value, err := r.store.Get(ctx, conn.OrganizationKey, conn.Key, conn.SecretRef)
	if err != nil || len(value) == 0 {
		return nil, "", ErrCredentialUnavailable
	}
	return value, conn.ConfigString("kubeContext"), nil
}
