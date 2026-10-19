package v03run

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestListOutputRefsUsesOnlyMetadataAndRejectsIncompleteLists(t *testing.T) {
	const group = "0123456789abcdef/"
	const id = "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct {
		name                string
		rootCode, groupCode int
		root, child         string
		wantErr             bool
		count               int
	}{
		{"empty404", 404, 0, "", "", false, 0},
		{"complete", 200, 200, `{"data":{"keys":["` + group + `"]}}`, `{"data":{"keys":["` + id + `"]}}`, false, 1},
		{"denied", 403, 0, "", "", true, 0},
		{"server error", 500, 0, "", "", true, 0},
		{"malformed", 200, 0, `{`, "", true, 0},
		{"missing keys", 200, 0, `{"data":{}}`, "", true, 0},
		{"unsafe group", 200, 0, `{"data":{"keys":["../"]}}`, "", true, 0},
		{"group disappears", 200, 404, `{"data":{"keys":["` + group + `"]}}`, "", true, 0},
		{"group denied", 200, 403, `{"data":{"keys":["` + group + `"]}}`, "", true, 0},
		{"unsafe object", 200, 200, `{"data":{"keys":["` + group + `"]}}`, `{"data":{"keys":["../"]}}`, true, 0},
		{"duplicate", 200, 200, `{"data":{"keys":["` + group + `"]}}`, `{"data":{"keys":["` + id + `","` + id + `"]}}`, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "LIST" || !strings.HasPrefix(r.URL.Path, "/v1/secret/metadata/orchestrator/outputs/") || r.Header.Get("X-Vault-Token") != "scoped" {
					t.Errorf("unexpected non-metadata request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(403)
					return
				}
				code, body := tc.rootCode, tc.root
				if strings.HasSuffix(r.URL.Path, group) {
					code, body = tc.groupCode, tc.child
				}
				w.WriteHeader(code)
				fmt.Fprint(w, body)
			}))
			defer s.Close()
			refs, err := (Vault{Address: s.URL, Mount: "secret", Token: "scoped"}).ListOutputRefs(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tc.wantErr)
			}
			if !tc.wantErr && len(refs) != tc.count {
				t.Fatalf("refs=%v", refs)
			}
			if tc.count == 1 && !reflect.DeepEqual(refs, []string{"secret://vault/secret/orchestrator/outputs/" + group + id}) {
				t.Fatalf("refs=%v", refs)
			}
		})
	}
}

func TestOutputInventoryProtectsBaselineAndBlocksUnknownObjects(t *testing.T) {
	for _, tc := range []struct {
		name           string
		live, owned    []string
		after, wantErr bool
	}{
		{"owned before cleanup", []string{"protected", "owned"}, []string{"owned"}, false, false},
		{"retry partially cleaned", []string{"protected"}, []string{"owned"}, false, false},
		{"unknown object", []string{"protected", "unknown"}, []string{"owned"}, false, true},
		{"baseline missing", []string{"owned"}, []string{"owned"}, false, true},
		{"baseline deletion attempted", []string{"protected"}, []string{"protected"}, false, true},
		{"clean", []string{"protected"}, []string{"owned"}, true, false},
		{"owned still exists", []string{"protected", "owned"}, []string{"owned"}, true, true},
		{"unknown after cleanup", []string{"protected", "unknown"}, []string{}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := CheckOutputInventory([]string{"protected"}, tc.live, tc.owned, tc.after); (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
