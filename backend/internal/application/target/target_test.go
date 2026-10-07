package target_test

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/application/target"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st := store.New()
	ctx := context.Background()
	for _, c := range []application.Connection{
		{ID: "k", Key: "lab", OrganizationKey: "acme", Kind: application.ConnectionKubernetes, Status: application.ConnectionReady},
		{ID: "k2", Key: "verifying", OrganizationKey: "acme", Kind: application.ConnectionKubernetes, Status: application.ConnectionVerifying},
		{ID: "a", Key: "cloud", OrganizationKey: "acme", Kind: application.ConnectionAWS, Status: application.ConnectionReady, Config: map[string]any{"region": "eu-west-1"}},
	} {
		if err := st.SaveConnection(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func TestResolveRequiresAConsistentReadyTarget(t *testing.T) {
	st := newStore(t)
	good := environment.Environment{Key: "staging", ApplicationKey: "app", ConnectionKey: "lab", Profile: application.ProfileInternalK8s, RuntimeStatus: application.RuntimeReady, InfrastructureScope: environment.ScopeEnvironment}
	aws := environment.Environment{Key: "production", ApplicationKey: "app", ConnectionKey: "cloud", Profile: application.ProfileAWSEKS, Region: "eu-west-1", RuntimeStatus: application.RuntimePending, InfrastructureScope: environment.ScopeEnvironment}
	with := func(base environment.Environment, mutate func(*environment.Environment)) environment.Environment {
		mutate(&base)
		return base
	}
	cases := []struct {
		name string
		env  environment.Environment
		want error
	}{
		{"unconfigured", environment.Environment{Key: "staging", ApplicationKey: "app"}, target.ErrUnconfigured},
		{"kubernetes ready", good, nil},
		{"aws pending", aws, nil},
		{"kubernetes profile on an AWS connection", with(good, func(e *environment.Environment) { e.ConnectionKey = "cloud" }), target.ErrInconsistent},
		{"aws profile on a Kubernetes connection", with(aws, func(e *environment.Environment) { e.ConnectionKey = "lab" }), target.ErrInconsistent},
		{"aws without a pinned region", with(aws, func(e *environment.Environment) { e.Region = "" }), target.ErrInconsistent},
		{"aws pinned region differs from the connection", with(aws, func(e *environment.Environment) { e.Region = "us-east-1" }), target.ErrInconsistent},
		{"unknown profile", with(good, func(e *environment.Environment) { e.Profile = "gcp" }), target.ErrInconsistent},
		{"unconfigured status on a bound row", with(good, func(e *environment.Environment) { e.RuntimeStatus = application.RuntimeUnconfigured }), target.ErrInconsistent},
		{"unknown scope", with(good, func(e *environment.Environment) { e.InfrastructureScope = "SHARED" }), target.ErrInconsistent},
		{"connection not ready", with(good, func(e *environment.Environment) { e.ConnectionKey = "verifying" }), target.ErrConnectionNotReady},
	}
	for _, tc := range cases {
		conn, err := target.Resolve(context.Background(), st, "acme", tc.env)
		if tc.want == nil {
			if err != nil || conn.Key != tc.env.ConnectionKey {
				t.Fatalf("%s: %v %+v", tc.name, err, conn)
			}
			continue
		}
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
		if tc.want != target.ErrUnconfigured && conn.Key != "" {
			t.Fatalf("%s: a connection was returned with the error", tc.name)
		}
	}
	// A connection of another Organization is never resolved.
	if _, err := target.Resolve(context.Background(), st, "globex", good); err == nil {
		t.Fatal("foreign organization resolved a connection")
	}
}

// ADR-011: product code resolves the target through the Environment. The
// legacy Application target fields may only be touched by the persistence
// adapters (migration/preservation) and the domain type itself.
func TestProductCodeNeverReadsTheApplicationTarget(t *testing.T) {
	root := filepath.Join("..", "..", "..", "internal")
	allowed := []string{"adapters/store", "adapters/postgres", "domain/application", "ports/persistence/persistencetest"}
	fields := map[string]bool{"ConnectionKey": true, "Profile": true, "Region": true, "RuntimeStatus": true}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		slashed := filepath.ToSlash(path)
		for _, prefix := range allowed {
			if strings.Contains(slashed, "/internal/"+prefix+"/") || strings.Contains(slashed, "internal/"+prefix+"/") {
				return nil
			}
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || !fields[sel.Sel.Name] {
				return true
			}
			// Receiver names that denote an Application value.
			switch x := sel.X.(type) {
			case *ast.Ident:
				if x.Name == "app" || x.Name == "storedApp" {
					t.Errorf("%s reads Application.%s; resolve the Environment target instead", path, sel.Sel.Name)
				}
			case *ast.SelectorExpr:
				if x.Sel.Name == "App" {
					t.Errorf("%s reads App.%s; resolve the Environment target instead", path, sel.Sel.Name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
