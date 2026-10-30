package gitpublication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"orchestrator/internal/ports/credentials"
	port "orchestrator/internal/ports/delivery"
	gitport "orchestrator/internal/ports/gitpublication"
	"orchestrator/internal/ports/persistence"
)

// Factory owns a private directory outside Git. Credential bytes live only in
// mode-0600 askpass files; URLs, argv, logs and persisted binding config carry no
// secrets. Runtime never inherits the host's Git helper or SSH agent.
type Factory struct {
	Root          string
	Credentials   credentials.Store
	Organizations persistence.OrganizationIdentityReader
	mu            sync.Mutex
	writers       map[string]*Writer
}
type GitCredential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func NewFactory(root string, store credentials.Store, orgs persistence.OrganizationIdentityReader) (*Factory, error) {
	if !filepath.IsAbs(root) || orgs == nil {
		return nil, ErrInvalid
	}
	for p := filepath.Clean(root); ; p = filepath.Dir(p) {
		if _, err := os.Lstat(filepath.Join(p, ".git")); err == nil {
			return nil, ErrInvalid
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, ErrInvalid
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, ErrInvalid
	}
	return &Factory{Root: root, Credentials: store, Organizations: orgs, writers: map[string]*Writer{}}, nil
}
func (f *Factory) ResolvePublisher(ctx context.Context, b port.BindingVersion) (gitport.Publisher, error) {
	repo, branch := b.Config["repositoryURL"], b.Config["branch"]
	parsed, err := url.Parse(repo)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !branchShape.MatchString(branch) || strings.Contains(branch, "..") {
		return nil, ErrInvalid
	}
	org, err := f.Organizations.GetOrganizationByID(ctx, b.OrganizationID)
	if err != nil {
		return nil, ErrInvalid
	}
	destination := sha256.Sum256([]byte(repo + "\x00" + branch))
	key := hex.EncodeToString(destination[:])
	f.mu.Lock()
	defer f.mu.Unlock()
	// Cache is scoped to binding version too: different credentials do not share
	// a process-local checkout even when their remote publication lease is common.
	identityBytes, err := json.Marshal([]any{b.OrganizationID, b.ID, b.Version, strings.TrimSpace(repo), branch, b.WriterCredentialRef, b.Config["caBundle"]})
	if err != nil {
		return nil, ErrInvalid
	}
	identity := sha256.Sum256(identityBytes)
	directory := filepath.Join(f.Root, hex.EncodeToString(identity[:]))
	cacheKey := directory
	if w := f.writers[cacheKey]; w != nil {
		return w, nil
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, ErrPublication
	}
	info, err := os.Lstat(directory)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, ErrInvalid
	}
	env := []string{"GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
	if ca := b.Config["caBundle"]; ca != "" {
		caPath := filepath.Join(directory, "ca.pem")
		if err := os.WriteFile(caPath, []byte(ca), 0o600); err != nil {
			return nil, ErrPublication
		}
		env = append(env, "GIT_SSL_CAINFO="+caPath)
	}
	if b.WriterCredentialRef != "" {
		if f.Credentials == nil || !f.Credentials.Durable() {
			return nil, ErrInvalid
		}
		raw, err := f.Credentials.Get(ctx, org.Key, b.ID, b.WriterCredentialRef)
		if err != nil {
			return nil, ErrPublication
		}
		var credential GitCredential
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&credential) != nil || credential.Username == "" || credential.Password == "" || strings.ContainsAny(credential.Username+credential.Password, "\x00\r\n") {
			return nil, ErrInvalid
		}
		file := filepath.Join(directory, "credential")
		if err := os.WriteFile(file, []byte(credential.Username+"\n"+credential.Password+"\n"), 0o600); err != nil {
			return nil, ErrPublication
		}
		script := filepath.Join(directory, "askpass.sh")
		body := "#!/bin/sh\ncase \"$1\" in *Username*) sed -n '1p' \"$ORCH_GIT_CREDENTIAL_FILE\" ;; *) sed -n '2p' \"$ORCH_GIT_CREDENTIAL_FILE\" ;; esac\n"
		if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
			return nil, ErrPublication
		}
		env = append(env, "GIT_ASKPASS="+script, "ORCH_GIT_CREDENTIAL_FILE="+file)
	}
	clone := filepath.Join(directory, "repository")
	if _, err := os.Stat(filepath.Join(clone, ".git")); os.IsNotExist(err) {
		cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-c", "credential.helper=", "clone", "--single-branch", "--branch", branch, "--", repo, clone)
		cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + directory}, env...)
		if err := cmd.Run(); err != nil {
			return nil, ErrPublication
		}
	}
	writer, err := New(clone, key, append(env, "HOME="+directory))
	if err != nil {
		return nil, err
	}
	f.writers[cacheKey] = writer
	return writer, nil
}
