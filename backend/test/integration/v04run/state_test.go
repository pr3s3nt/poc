package v04run

import "testing"

func inst(descriptor, id, key string) Instance {
	return Instance{Descriptor: descriptor, ID: id, StateKey: key, Status: "READY", Scope: "SHARED"}
}

func TestSharedServicesMustBeExactlyOneOfEach(t *testing.T) {
	good := []Instance{inst("k8s-namespace.default#e", "n", "k0"), inst("postgres.in-cluster#shared.db", "p", "k1"), inst("redis.in-cluster#shared.cache", "r", "k2")}
	if err := CheckSingleton(good); err != nil {
		t.Fatal(err)
	}
	if err := CheckSingleton(append(good, inst("postgres.in-cluster#shared.db2", "p2", "k3"))); err == nil {
		t.Fatal("a second PostgreSQL instance (a duplicate) must fail")
	}
	if err := CheckSingleton(good[:2]); err == nil {
		t.Fatal("a missing Redis instance must fail")
	}
}

func TestSameInstancesDetectsReplacementDuplicationAndLoss(t *testing.T) {
	before := []Instance{inst("postgres.in-cluster#shared.db", "p", "k1"), inst("redis.in-cluster#shared.cache", "r", "k2")}
	if err := SameInstances(before, append([]Instance(nil), before...)); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]Instance{
		"replaced": {inst("postgres.in-cluster#shared.db", "p-new", "k1"), before[1]},
		"rekeyed":  {inst("postgres.in-cluster#shared.db", "p", "kX"), before[1]},
		"lost":     {before[0]},
		"new":      {before[0], before[1], inst("redis.in-cluster#shared.other", "x", "k9")},
	}
	for name, after := range cases {
		if err := SameInstances(before, after); err == nil {
			t.Fatalf("%s must be reported", name)
		}
	}
}
