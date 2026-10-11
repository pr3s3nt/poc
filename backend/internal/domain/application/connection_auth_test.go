package application

import "testing"

// T22: Kubernetes authentication is explicit KUBECONFIG. HOST_CONTEXT and a
// missing type (whatever the reference looks like) fail closed; only the
// process-configured AWS seed reference infers its type.
func TestWithLegacyDefaultsNeverInfersKubernetesAuthentication(t *testing.T) {
	for name, conn := range map[string]Connection{
		"host context":          {Key: "a", Kind: ConnectionKubernetes, AuthenticationType: "HOST_CONTEXT", SecretRef: "host-kube-context://a"},
		"missing, host ref":     {Key: "b", Kind: ConnectionKubernetes, SecretRef: "host-kube-context://b"},
		"missing, seed ref":     {Key: "c", Kind: ConnectionKubernetes, SecretRef: "secret://connections/c"},
		"missing, empty ref":    {Key: "d", Kind: ConnectionKubernetes},
		"kubeconfig for AWS":    {Key: "e", Kind: ConnectionAWS, AuthenticationType: AuthKubeconfig},
		"missing AWS, host ref": {Key: "f", Kind: ConnectionAWS, SecretRef: "host-kube-context://f"},
	} {
		if _, err := conn.WithLegacyDefaults(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	aws, err := Connection{Key: "cloud", Kind: ConnectionAWS, SecretRef: "secret://connections/cloud"}.WithLegacyDefaults()
	if err != nil || aws.AuthenticationType != AuthAWSAccessKey || aws.Name != "cloud" {
		t.Fatalf("AWS seed shape: %+v, %v", aws, err)
	}
	if kube, err := (Connection{Key: "k", Kind: ConnectionKubernetes, AuthenticationType: AuthKubeconfig}).WithLegacyDefaults(); err != nil || !kube.CredentialBacked() {
		t.Fatalf("KUBECONFIG: %+v, %v", kube, err)
	}
}
