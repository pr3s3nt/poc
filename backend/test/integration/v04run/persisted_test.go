package v04run

import (
	"reflect"
	"testing"
)

func TestScanTextFindsCredentialMaterialButNotLegitimateNames(t *testing.T) {
	clean := `{"cluster":"orch-shop","namespace":"orch-ns","provider":"eks","region":"us-east-1","connection":"aws-a","placement":{"account":"aws-a"},"outputs":["kubeconfig","kubeContext","endpoint"]}`
	if got := ScanText(clean); len(got) != 0 {
		t.Fatalf("a safe target and module output NAMES are not credentials: %v", got)
	}
	dirty := map[string]string{
		"credential key":      `{"token":"abc"}`,
		"certificate":         `BEGIN CERTIFICATE`,
		"signed EKS token":    `k8s-aws-v1.aHR0cHM6Ly9zdHM`,
		"cluster endpoint":    `https://ABCDEF.gr7.us-east-1.eks.amazonaws.com`,
		"AWS access key":      `AKIAABCDEFGHIJKLMNOP`,
		"temporary file path": `/tmp/orch-kube-12345/config`,
	}
	for kind, text := range dirty {
		if got := ScanText(text); !reflect.DeepEqual(got, []string{kind}) {
			t.Errorf("%s: %v", kind, got)
		}
	}
}
