package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/authentication"
)

func TestOIDCConfigurationFailsClosedBeforeDiscovery(t *testing.T) {
	st := store.New()
	auth := authentication.NewService(st)
	ctx := context.Background()
	if e := configureOIDC(ctx, st, auth, ""); e != nil || auth.OIDCEnabled() {
		t.Fatal("local mode", e)
	}
	file := filepath.Join(t.TempDir(), "oidc.json")
	for _, tc := range []struct {
		name, content string
		mode          os.FileMode
	}{
		{"unsafe permissions", `{"clientSecret":"private"}`, 0644},
		{"invalid JSON", `{`, 0600},
		{"missing organization", `{"organizationKey":"unknown","issuer":"http://127.0.0.1:1","clientId":"platform","clientSecret":"private","publicUrl":"http://console.test","allowHttp":true}`, 0600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			os.Remove(file)
			if e := os.WriteFile(file, []byte(tc.content), tc.mode); e != nil {
				t.Fatal(e)
			}
			if e := configureOIDC(ctx, st, auth, file); e == nil || auth.OIDCEnabled() {
				t.Fatal("invalid configuration enabled SSO")
			}
		})
	}
}
