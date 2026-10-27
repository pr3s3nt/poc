// prepare-provider-mirror downloads release-pinned provider archives during
// image build. Runtime never uses this tool or downloads providers.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"orchestrator/internal/adapters/terraform/registry"
)

func main() {
	dir := flag.String("dir", "", "destination filesystem mirror directory")
	platform := flag.String("platform", "linux_"+runtime.GOARCH, "provider platform (linux_amd64 or linux_arm64)")
	flag.Parse()
	if *dir == "" || (*platform != "linux_amd64" && *platform != "linux_arm64") {
		fmt.Fprintln(os.Stderr, "mirror: specify -dir and a supported Linux platform")
		os.Exit(1)
	}
	release, err := registry.DefaultRelease()
	if err == nil {
		client := &http.Client{Timeout: 15 * time.Minute}
		for _, provider := range release.Providers {
			err = download(context.Background(), client, "https://releases.hashicorp.com", *dir, *platform, provider)
			if err != nil {
				break
			}
			fmt.Printf("verified %s %s %s\n", provider.Source, provider.Version, *platform)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var safePart = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
var exactVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func download(ctx context.Context, client *http.Client, origin, dir, platform string, p registry.ReleaseProvider) error {
	parts := strings.Split(p.Source, "/")
	if len(parts) != 3 || parts[0] != "registry.terraform.io" || parts[1] != "hashicorp" || !safePart.MatchString(parts[2]) || !exactVersion.MatchString(p.Version) || (platform != "linux_amd64" && platform != "linux_arm64") {
		return fmt.Errorf("mirror: unsupported release provider or platform")
	}
	name := "terraform-provider-" + parts[2]
	archive := name + "_" + p.Version + "_" + platform + ".zip"
	target := filepath.Join(dir, filepath.FromSlash(p.Source))
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	// Publish only verified bytes, never a partial download at the mirror path.
	f, err := os.CreateTemp(target, ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/"+name+"/"+p.Version+"/"+archive, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("mirror: download %s: %w", p.Source, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("mirror: download %s: HTTP %d", p.Source, response.StatusCode)
	}
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, hash), response.Body); err != nil {
		return err
	}
	if !slices.Contains(p.Hashes, "zh:"+hex.EncodeToString(hash.Sum(nil))) {
		return fmt.Errorf("mirror: archive checksum does not match release for %s %s", p.Source, p.Version)
	}
	if err := f.Chmod(0o444); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(target, archive))
}
