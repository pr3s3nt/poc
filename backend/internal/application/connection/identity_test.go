package connection_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/internal/adapters/credentialmemory"
	"orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
)

type connections map[string]application.Connection

func (c connections) GetConnection(_ context.Context, org, key string) (application.Connection, error) {
	conn, ok := c[org+"/"+key]
	if !ok {
		return application.Connection{}, errors.New("missing")
	}
	return conn, nil
}

func awsConnection(secretRef string) application.Connection {
	return application.Connection{Key: "acct", OrganizationKey: "acme", Kind: application.ConnectionAWS, AuthenticationType: application.AuthAWSAccessKey, Status: application.ConnectionReady, SecretRef: secretRef, Config: map[string]any{"accountId": "123456789012", "region": "us-east-1"}}
}

func TestResolveAWSReadsOnlyAScopedReadyAccessKey(t *testing.T) {
	store := credentialmemory.New()
	ctx := context.Background()
	ref, err := store.Put(ctx, "acme", "acct", []byte(`{"accessKeyId":"AKID","secretAccessKey":"SECRET"}`))
	if err != nil {
		t.Fatal(err)
	}
	resolver := connection.NewCredentialResolver(connections{"acme/acct": awsConnection(ref)}, store)
	got, err := resolver.ResolveAWS(ctx, "acme", "acct")
	if err != nil || got.AccessKeyID != "AKID" || got.SecretAccessKey != "SECRET" || got.AccountID != "123456789012" || got.Region != "us-east-1" {
		t.Fatalf("resolved %+v: %v", got, err)
	}
	// Foreign organization, missing Connection and wrong kind fail closed.
	if _, err := resolver.ResolveAWS(ctx, "globex", "acct"); !errors.Is(err, connection.ErrCredentialUnavailable) {
		t.Fatalf("foreign organization: %v", err)
	}
	notReady := awsConnection(ref)
	notReady.Status = application.ConnectionVerifying
	noAccount := awsConnection(ref)
	noAccount.Config = map[string]any{"region": "us-east-1"}
	wrongKind := awsConnection(ref)
	wrongKind.Kind, wrongKind.AuthenticationType = application.ConnectionKubernetes, application.AuthKubeconfig
	for name, conn := range map[string]application.Connection{"not ready": notReady, "no pinned account": noAccount, "wrong kind": wrongKind} {
		r := connection.NewCredentialResolver(connections{"acme/acct": conn}, store)
		if _, err := r.ResolveAWS(ctx, "acme", "acct"); !errors.Is(err, connection.ErrCredentialUnavailable) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
	// A value with unknown fields is not an access key.
	extra, _ := store.Put(ctx, "acme", "acct", []byte(`{"accessKeyId":"AKID","secretAccessKey":"S","kubeconfig":"x"}`))
	if _, err := connection.NewCredentialResolver(connections{"acme/acct": awsConnection(extra)}, store).ResolveAWS(ctx, "acme", "acct"); !errors.Is(err, connection.ErrCredentialUnavailable) {
		t.Fatalf("value with extra fields: %v", err)
	}
	if _, err := connection.NewCredentialResolver(connections{"acme/acct": awsConnection(ref)}, nil).ResolveAWS(ctx, "acme", "acct"); !errors.Is(err, connection.ErrCredentialUnavailable) {
		t.Fatalf("no credential store: %v", err)
	}
}
