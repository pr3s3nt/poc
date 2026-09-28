package kubernetes

import "testing"

func TestHasBundleKeys(t *testing.T) {
	keys := map[string]map[string]string{"main": {"API_TOKEN": "main_API_TOKEN", "DB_HOST": "main_DB_HOST"}}
	for _, tc := range []struct {
		name   string
		secret map[string]any
		want   bool
	}{
		{"complete", map[string]any{"data": map[string]any{"main_API_TOKEN": "opaque", "main_DB_HOST": "opaque"}}, true},
		{"missing", map[string]any{"data": map[string]any{"main_API_TOKEN": "opaque"}}, false},
		{"no-data", map[string]any{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasBundleKeys(tc.secret, keys); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
