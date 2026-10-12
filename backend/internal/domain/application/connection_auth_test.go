package application

import "testing"

// T22/T28: Connection metadata is explicit. A missing name, a missing type,
// HOST_CONTEXT and a kind-incompatible type are all rejected; nothing is inferred
// from the secret reference.
func TestConnectionValidateRequiresExplicitMetadata(t *testing.T) {
	for name, conn := range map[string]Connection{
		"host context":           {Key: "a", Name: "a", Kind: ConnectionKubernetes, AuthenticationType: "HOST_CONTEXT", SecretRef: "host-kube-context://a"},
		"missing, host ref":      {Key: "b", Name: "b", Kind: ConnectionKubernetes, SecretRef: "host-kube-context://b"},
		"missing, seed ref":      {Key: "c", Name: "c", Kind: ConnectionKubernetes, SecretRef: "secret://connections/c"},
		"missing, empty ref":     {Key: "d", Name: "d", Kind: ConnectionKubernetes},
		"kubeconfig for AWS":     {Key: "e", Name: "e", Kind: ConnectionAWS, AuthenticationType: AuthKubeconfig},
		"missing AWS seed ref":   {Key: "f", Name: "f", Kind: ConnectionAWS, SecretRef: "secret://connections/f"},
		"missing name":           {Key: "g", Kind: ConnectionKubernetes, AuthenticationType: AuthKubeconfig},
		"AWS key for Kubernetes": {Key: "h", Name: "h", Kind: ConnectionKubernetes, AuthenticationType: AuthAWSAccessKey},
	} {
		if err := conn.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	aws := Connection{Key: "cloud", Name: "cloud", Kind: ConnectionAWS, AuthenticationType: AuthAWSAccessKey, SecretRef: "secret://connections/cloud"}
	if err := aws.Validate(); err != nil || aws.CredentialBacked() {
		t.Fatalf("AWS: %+v, %v", aws, err)
	}
	if kube := (Connection{Key: "k", Name: "k", Kind: ConnectionKubernetes, AuthenticationType: AuthKubeconfig}); kube.Validate() != nil || !kube.CredentialBacked() {
		t.Fatalf("KUBECONFIG: %+v", kube)
	}
}
