// Command orchestrator runs the orchestrator API and serves the Web Console bundle.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"orchestrator/internal/bootstrap"
	"orchestrator/internal/seed"
)

func main() {
	opts := seed.Defaults()
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	addrFile := flag.String("addr-file", "", "optional file that receives the resolved listen address")
	uiDir := flag.String("ui-dir", "../frontend/dist", "directory holding the Web Console production bundle")
	statePath := flag.String("state", "", "optional path of the JSON state snapshot")
	adapters := flag.String("adapters", "fake", "executor adapters: fake, kubernetes or aws")
	profile := flag.String("profile", opts.Profile, "seed profile: local, test, or production")
	namespace := flag.String("namespace", opts.NamespaceIdentity, "namespace identity of the internal-k8s environment")
	cloudNamespace := flag.String("cloud-namespace", opts.CloudNamespaceIdentity, "namespace identity of the aws-eks environment")
	kubeContext := flag.String("kube-context", opts.KubeContext, "kubectl context of the registered cluster")
	cluster := flag.String("cluster", opts.ClusterName, "registered cluster name")
	region := flag.String("region", "", "AWS region; the aws-eks application is registered only when it is set")
	accountID := flag.String("account-id", "", "AWS account id for the aws-eks application")
	runID := flag.String("run-id", "", "verification run id, used in cloud resource names and tags")
	owner := flag.String("owner", "", "owner tag for cloud resources")
	ttl := flag.Duration("ttl", 2*time.Hour, "expires-at tag horizon for cloud resources")
	frontendImage := flag.String("frontend-image", opts.Images.Frontend, "acceptance frontend image")
	backendImage := flag.String("backend-image", opts.Images.Backend, "acceptance backend image")
	workerImage := flag.String("worker-image", opts.Images.Worker, "acceptance worker image")
	terraformRoot := flag.String("terraform-root", "", "directory holding Terraform run state")
	terraformCache := flag.String("terraform-plugin-cache", os.Getenv("TF_PLUGIN_CACHE_DIR"), "Terraform provider cache directory")
	kubectlPath := flag.String("kubectl", "kubectl", "kubectl binary")
	terraformPath := flag.String("terraform", "terraform", "terraform binary")
	flag.Parse()

	opts.NamespaceIdentity = *namespace
	opts.Profile = *profile
	opts.CloudNamespaceIdentity = *cloudNamespace
	opts.KubeContext = *kubeContext
	opts.ClusterName = *cluster
	opts.Region = *region
	opts.AccountID = *accountID
	opts.RunID = *runID
	opts.Images.Frontend = *frontendImage
	opts.Images.Backend = *backendImage
	opts.Images.Worker = *workerImage

	ownerTag := *owner
	if ownerTag == "" {
		ownerTag = "orchestrator"
	}
	tags := map[string]string{
		"project":     "orchestrator",
		"owner":       ownerTag,
		"environment": opts.EnvironmentKey,
		"run-id":      *runID,
		"expires-at":  time.Now().UTC().Add(*ttl).Format(time.RFC3339),
		"managed-by":  "orchestrator-verification",
	}

	mode := bootstrap.AdapterMode(*adapters)
	if mode == bootstrap.AdapterAWS {
		if *region == "" {
			log.Print("orchestrator: -adapters aws needs -region")
			os.Exit(1)
		}
		if *terraformRoot == "" {
			log.Print("orchestrator: -adapters aws needs -terraform-root")
			os.Exit(1)
		}
		if *runID == "" {
			log.Print("orchestrator: -adapters aws needs -run-id so cloud resources carry a run id tag")
			os.Exit(1)
		}
	}

	app, err := bootstrap.Build(context.Background(), bootstrap.Options{
		Seed:                 opts,
		UIDir:                *uiDir,
		StatePath:            *statePath,
		Adapters:             mode,
		KubectlPath:          *kubectlPath,
		TerraformPath:        *terraformPath,
		TerraformRoot:        *terraformRoot,
		TerraformPluginCache: *terraformCache,
		Region:               *region,
		Tags:                 tags,
	})
	if err != nil {
		log.Printf("orchestrator: startup failed: %v", err)
		os.Exit(1)
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Printf("orchestrator: listen %s: %v", *addr, err)
		os.Exit(1)
	}
	resolved := listener.Addr().String()
	if *addrFile != "" {
		if err := os.WriteFile(*addrFile, []byte(resolved), 0o600); err != nil {
			log.Printf("orchestrator: write addr file: %v", err)
			os.Exit(1)
		}
	}

	server := &http.Server{Handler: app.Server, ReadHeaderTimeout: 10 * time.Second}
	fmt.Printf("orchestrator listening on http://%s (api /api/v1/, console /ui/)\n", resolved)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Printf("orchestrator: %v", err)
		os.Exit(1)
	}
}
