package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	domain "orchestrator/internal/domain/delivery"
)

func TestEnsurePrivateRepositoryConvergesAndPinsIdentity(t *testing.T) {
	for _, outcome := range []string{"created", "conflict", "lost-response"} {
		t.Run(outcome, func(t *testing.T) {
			var mu sync.Mutex
			var remote map[string]any
			posts := 0
			in := domain.ApplicationRepository{ID: "allocation", ApplicationID: "app", OrganizationID: "org", BindingID: "binding", BindingVersion: 1, Name: "orch-app-id", Marker: "private-allocation-marker", Status: "PENDING", Revision: 1, CreatedAt: time.Now()}
			var base string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if r.Header.Get("Authorization") != "Bearer secret-token" {
					t.Error("missing private authentication")
					w.WriteHeader(401)
					return
				}
				switch r.URL.Path {
				case "/user":
					json.NewEncoder(w).Encode(map[string]any{"login": "operator"})
				case "/users/platform":
					json.NewEncoder(w).Encode(map[string]any{"login": "platform", "type": "Organization"})
				case "/user/memberships/orgs/platform":
					json.NewEncoder(w).Encode(map[string]any{"state": "active"})
				case "/orgs/platform/repos":
					posts++
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if body["private"] != true || body["auto_init"] != true || body["description"] != in.Marker || body["name"] != in.Name {
						t.Error("unsafe repository creation")
					}
					remote = map[string]any{"id": 123, "name": in.Name, "full_name": "platform/" + in.Name, "clone_url": base + "/platform/" + in.Name + ".git", "default_branch": "trunk", "description": in.Marker, "private": true}
					if outcome == "lost-response" {
						c, _, _ := w.(http.Hijacker).Hijack()
						c.Close()
						return
					}
					if outcome == "conflict" {
						w.WriteHeader(422)
						return
					}
					w.WriteHeader(201)
				case "/repos/platform/" + in.Name:
					if remote == nil {
						w.WriteHeader(404)
						return
					}
					json.NewEncoder(w).Encode(remote)
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			base = server.URL
			h := &Host{policy: domain.RepositoryPolicy{Enabled: true, Host: "github", APIBase: base, CloneBase: base, Owner: "platform", Prefix: "orch-", Root: "environments"}, token: "secret-token", client: server.Client()}
			if err := h.Verify(context.Background()); err != nil {
				t.Fatal(err)
			}
			ready, err := h.Ensure(context.Background(), in)
			if err != nil || ready.Branch != "trunk" || ready.RemoteID != "123" {
				t.Fatal("provision", err)
			}
			again, err := h.Ensure(context.Background(), in)
			if err != nil || again != ready || posts != 1 {
				t.Fatal("retry created another repository", err)
			}
			mu.Lock()
			remote["id"] = 456
			mu.Unlock()
			if _, err := h.Ensure(context.Background(), ready); !errors.Is(err, ErrIdentity) {
				t.Fatal("remote replacement adopted", err)
			}
			mu.Lock()
			remote["description"] = "someone else's repository"
			mu.Unlock()
			if _, err := h.Ensure(context.Background(), in); !errors.Is(err, ErrIdentity) {
				t.Fatal("foreign repository adopted", err)
			}
			if posts != 1 {
				t.Fatal("identity mismatch caused creation")
			}
		})
	}
}

func TestReadyRepositoryIsNeverRecreated(t *testing.T) {
	posts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	h := &Host{policy: domain.RepositoryPolicy{APIBase: server.URL, Owner: "platform"}, client: server.Client()}
	in := domain.ApplicationRepository{ID: "allocation", ApplicationID: "app", OrganizationID: "org", BindingID: "binding", BindingVersion: 1, Name: "orch-app", Marker: "marker", Status: "READY", RemoteID: "123", CloneURL: "https://github.com/platform/orch-app.git", Branch: "trunk", Revision: 2, CreatedAt: time.Now()}
	if _, err := h.Ensure(context.Background(), in); !errors.Is(err, ErrIdentity) || posts != 0 {
		t.Fatal("recreated missing ready repo", err)
	}
}
