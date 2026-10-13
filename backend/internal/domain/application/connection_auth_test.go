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

func TestConnectionAllowedRegions(t *testing.T) {
	cases := map[string]struct {
		config map[string]any
		want   []string
		allows map[string]bool
	}{
		"list from memory":     {map[string]any{"region": "us-east-1", "allowedRegions": []string{"us-east-1", "eu-west-1"}}, []string{"us-east-1", "eu-west-1"}, map[string]bool{"eu-west-1": true, "ap-south-1": false}},
		"list after JSON":      {map[string]any{"region": "us-east-1", "allowedRegions": []any{"us-east-1", "eu-west-1"}}, []string{"us-east-1", "eu-west-1"}, map[string]bool{"us-east-1": true, "ap-south-1": false}},
		"legacy single region": {map[string]any{"region": "eu-west-1"}, []string{"eu-west-1"}, map[string]bool{"eu-west-1": true, "us-east-1": false}},
		"malformed list":       {map[string]any{"region": "eu-west-1", "allowedRegions": []any{"us-east-1", 7}}, []string{"eu-west-1"}, map[string]bool{"us-east-1": false}},
		"none":                 {map[string]any{}, nil, map[string]bool{"us-east-1": false, "": false}},
	}
	for name, tc := range cases {
		conn := Connection{Config: tc.config}
		got := conn.AllowedRegions()
		if len(got) != len(tc.want) {
			t.Errorf("%s: %v", name, got)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: %v", name, got)
			}
		}
		for region, want := range tc.allows {
			if conn.AllowsRegion(region) != want {
				t.Errorf("%s: AllowsRegion(%q) = %v", name, region, !want)
			}
		}
	}
	// The returned list is a copy.
	conn := Connection{Config: map[string]any{"allowedRegions": []string{"us-east-1"}}}
	conn.AllowedRegions()[0] = "x"
	if conn.AllowedRegions()[0] != "us-east-1" {
		t.Fatal("AllowedRegions exposes the stored slice")
	}
}
