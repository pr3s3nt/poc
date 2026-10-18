package main

import (
	"reflect"
	"testing"
)

func TestV03CommandsAreRegisteredAndNeedAnExplicitContext(t *testing.T) {
	for _, name := range []string{"v03-preflight", "v03-record-owned", "v03-workload", "v03-absent", "v03-reap"} {
		if extra[name] == nil {
			t.Fatalf("%s is not registered", name)
		}
		// Without -kubeconfig/-context every command refuses before it touches anything.
		if code := extra[name](nil); code == 0 {
			t.Fatalf("%s accepted a missing host context", name)
		}
	}
}

func TestV03VaultCleanFailsWithoutAProvenKeySource(t *testing.T) {
	if extra["v03-vault-clean"] == nil {
		t.Fatal("v03-vault-clean is not registered")
	}
	if code := extra["v03-vault-clean"](nil); code == 0 {
		t.Fatal("v03-vault-clean accepted a missing token file")
	}
}

func TestSplitNamesTrimsAndDropsBlanks(t *testing.T) {
	if got := splitNames(" a, b ,,c "); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("got %v", got)
	}
}
