package application

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

// Application carries identity and current metadata only; execution and store
// bindings belong to the Environment (T26).
func TestApplicationSerializesIdentityOnly(t *testing.T) {
	b, err := json.Marshal(Application{ID: "a", Key: "a", OrganizationKey: "acme", Name: "A", Subdomain: "a", Version: 2})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	var got []string
	for k := range fields {
		got = append(got, k)
	}
	sort.Strings(got)
	want := []string{"id", "key", "name", "organizationKey", "subdomain", "version"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("application JSON fields = %v, want %v", got, want)
	}
}

func TestApplicationValidateIdentityOnly(t *testing.T) {
	if err := (Application{Key: "ok", Subdomain: "ok"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Application{Key: "Bad"}).Validate(); err == nil {
		t.Fatal("invalid key accepted")
	}
	if err := (Application{Key: "ok", Subdomain: "-x"}).Validate(); err == nil {
		t.Fatal("invalid subdomain accepted")
	}
}
