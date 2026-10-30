package fleet

import "testing"

func validConfig() map[string]string {
	return map[string]string{"agentNamespace": "cattle-fleet-system", "workspace": "fleet-local", "controllerNamespace": "cattle-fleet-system", "controllerName": "fleet-controller", "targetCluster": "local", "targetClusterUID": "cluster-uid", "targetAccountKey": "kind-reconciliation", "repositoryURL": "https://example.com/generated.git", "branch": "main", "root": "generated", "readSecret": "git-read", "serviceAccount": "workload-delivery", "releaseNamespace": "fleet-releases"}
}
func TestBindingRejectsCredentialsAndEscapingPublicationPaths(t *testing.T) {
	if _, err := ParseConfig(validConfig()); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(map[string]string){
		"embedded credential":         func(c map[string]string) { c["repositoryURL"] = "https://token@example.com/repo.git" },
		"raw token":                   func(c map[string]string) { c["token"] = "must-not-be-config" },
		"parent path":                 func(c map[string]string) { c["root"] = "../foreign" },
		"revision expression":         func(c map[string]string) { c["branch"] = "main..other" },
		"target selector":             func(c map[string]string) { c["clusterSelector"] = "*" },
		"host kubecontext":            func(c map[string]string) { c["kubeContext"] = "default" },
		"unknown cluster incarnation": func(c map[string]string) { c["targetClusterUID"] = "" },
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig()
			change(c)
			if _, err := ParseConfig(c); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}
