package tfexec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/terraformexec"
)

type baselineStore struct{ values map[string][]byte }

func (s *baselineStore) SaveVerification(_ context.Context, handle, _ string, value []byte) error {
	if s.values == nil {
		s.values = map[string][]byte{}
	}
	if _, exists := s.values[handle]; !exists {
		s.values[handle] = append([]byte(nil), value...)
	}
	return nil
}
func (s *baselineStore) LoadVerification(_ context.Context, handle, _ string) ([]byte, bool, error) {
	v, ok := s.values[handle]
	return append([]byte(nil), v...), ok, nil
}

type baselineInspector struct {
	inspection terraformexec.NamespaceInspection
}

func (i baselineInspector) Inspect(context.Context, []string, string) (terraformexec.NamespaceInspection, error) {
	return i.inspection, nil
}

type baselineSequenceInspector struct {
	inspections []terraformexec.NamespaceInspection
	calls       int
}

func (i *baselineSequenceInspector) Inspect(context.Context, []string, string) (terraformexec.NamespaceInspection, error) {
	index := i.calls
	i.calls++
	if index >= len(i.inspections) {
		index = len(i.inspections) - 1
	}
	return i.inspections[index], nil
}

const nsUID = "uid-ns"

func obj(kind, name, uid string, owners ...terraformexec.NamespaceOwner) terraformexec.NamespaceObject {
	return terraformexec.NamespaceObject{Kind: kind, Name: name, UID: uid, Owners: owners}
}

func owner(kind, name, uid string) terraformexec.NamespaceOwner {
	return terraformexec.NamespaceOwner{Kind: kind, Name: name, UID: uid}
}

func generated(o terraformexec.NamespaceObject) terraformexec.NamespaceObject {
	o.Managers = []string{controllerManager}
	return o
}

// The ownership policy of a namespace delete is by UID and recorded
// provenance, never by name or shape alone.
func TestJudgeNamespaceAccountsOnlyForProvenObjects(t *testing.T) {
	own := []terraformexec.StateResource{
		{Address: "kubernetes_namespace_v1.ns", Type: "kubernetes_namespace_v1", ID: "orc-ns", UID: nsUID},
		{Address: "kubernetes_stateful_set_v1.redis", Type: "kubernetes_stateful_set_v1", ID: "orc-ns/redis", UID: "uid-sts"},
		{Address: "kubernetes_service_v1.redis", Type: "kubernetes_service_v1", ID: "orc-ns/redis", UID: "uid-svc"},
		{Address: "kubernetes_secret_v1.pw", Type: "kubernetes_secret_v1", ID: "orc-ns/pw", UID: "uid-pw"},
		{Address: "kubernetes_secret_v1.other", Type: "kubernetes_secret_v1", ID: "another-ns/elsewhere", UID: "uid-else"},
	}
	rootCA := generated(terraformexec.NamespaceObject{Kind: "ConfigMap", Name: "kube-root-ca.crt", UID: "uid-ca", DataKeys: []string{"ca.crt"}, AnnotationKeys: []string{"kubernetes.io/description"}})
	defaultSA := generated(terraformexec.NamespaceObject{Kind: "ServiceAccount", Name: "default", UID: "uid-sa"})
	sts := obj("StatefulSet", "redis", "uid-sts")
	svc := obj("Service", "redis", "uid-svc")
	with := func(o terraformexec.NamespaceObject, f func(*terraformexec.NamespaceObject)) terraformexec.NamespaceObject {
		f(&o)
		return o
	}
	cases := map[string]struct {
		objects []terraformexec.NamespaceObject
		foreign []string
	}{
		"empty":                                   {nil, nil},
		"genuine generated defaults":              {[]terraformexec.NamespaceObject{rootCA, defaultSA}, nil},
		"own objects with state UIDs":             {[]terraformexec.NamespaceObject{sts, obj("Secret", "pw", "uid-pw")}, nil},
		"owned children, exact UID":               {[]terraformexec.NamespaceObject{sts, obj("Pod", "redis-0", "uid-p", owner("StatefulSet", "redis", "uid-sts")), obj("ControllerRevision", "redis-1", "uid-cr", owner("StatefulSet", "redis", "uid-sts"))}, nil},
		"generated Endpoints of own Service":      {[]terraformexec.NamespaceObject{svc, generated(obj("Endpoints", "redis", "uid-ep")), obj("EndpointSlice", "redis-abc", "uid-es", owner("Service", "redis", "uid-svc"))}, nil},
		"standalone foreign Endpoints":            {[]terraformexec.NamespaceObject{svc, with(obj("Endpoints", "redis", "uid-ep"), func(o *terraformexec.NamespaceObject) { o.Managers = []string{"kubectl-create"} })}, []string{"Endpoints/redis"}},
		"foreign Endpoints without Service":       {[]terraformexec.NamespaceObject{with(obj("Endpoints", "custom", "uid-ep"), func(o *terraformexec.NamespaceObject) { o.Managers = []string{"kubectl-create"} })}, []string{"Endpoints/custom"}},
		"generated Endpoints without own Service": {[]terraformexec.NamespaceObject{generated(obj("Endpoints", "orphan", "uid-ep"))}, []string{"Endpoints/orphan"}},
		"foreign pod":                             {[]terraformexec.NamespaceObject{obj("Pod", "stranger", "uid-x")}, []string{"Pod/stranger"}},
		"foreign pvc and secret":                  {[]terraformexec.NamespaceObject{obj("PersistentVolumeClaim", "data", "uid-d"), obj("Secret", "token", "uid-t")}, []string{"PersistentVolumeClaim/data", "Secret/token"}},
		"own name with another UID (replaced)":    {[]terraformexec.NamespaceObject{obj("Secret", "pw", "uid-replaced")}, []string{"Secret/pw"}},
		"own name in another namespace":           {[]terraformexec.NamespaceObject{obj("Secret", "elsewhere", "uid-else")}, []string{"Secret/elsewhere"}},
		"owner UID mismatch (spoofed owner)":      {[]terraformexec.NamespaceObject{sts, obj("Pod", "p", "uid-p", owner("StatefulSet", "redis", "uid-spoofed"))}, []string{"Pod/p"}},
		"owner is a recreated same-name object":   {[]terraformexec.NamespaceObject{obj("StatefulSet", "redis", "uid-new"), obj("Pod", "p", "uid-p", owner("StatefulSet", "redis", "uid-sts"))}, []string{"Pod/p", "StatefulSet/redis"}},
		"child of foreign owner":                  {[]terraformexec.NamespaceObject{obj("Pod", "p", "uid-p", owner("ReplicaSet", "foreign", "uid-rs"))}, []string{"Pod/p"}},
		"partly owned stays foreign":              {[]terraformexec.NamespaceObject{sts, obj("Pod", "p", "uid-p", owner("StatefulSet", "redis", "uid-sts"), owner("Job", "x", "uid-j"))}, []string{"Pod/p"}},
		"root CA created by someone else":         {[]terraformexec.NamespaceObject{with(rootCA, func(o *terraformexec.NamespaceObject) { o.Managers = []string{"kubectl-create"} })}, []string{"ConfigMap/kube-root-ca.crt"}},
		"root CA edited by another manager":       {[]terraformexec.NamespaceObject{with(rootCA, func(o *terraformexec.NamespaceObject) { o.Managers = []string{controllerManager, "kubectl-edit"} })}, []string{"ConfigMap/kube-root-ca.crt"}},
		"root CA look-alike keys":                 {[]terraformexec.NamespaceObject{with(rootCA, func(o *terraformexec.NamespaceObject) { o.DataKeys = []string{"ca.crt", "extra"} })}, []string{"ConfigMap/kube-root-ca.crt"}},
		"root CA with custom label":               {[]terraformexec.NamespaceObject{with(rootCA, func(o *terraformexec.NamespaceObject) { o.LabelKeys = []string{"team"} })}, []string{"ConfigMap/kube-root-ca.crt"}},
		"root CA with an owner": {[]terraformexec.NamespaceObject{with(rootCA, func(o *terraformexec.NamespaceObject) {
			o.Owners = []terraformexec.NamespaceOwner{owner("Job", "x", "uid-j")}
		})}, []string{"ConfigMap/kube-root-ca.crt"}},
		"default SA with a secret":                {[]terraformexec.NamespaceObject{with(defaultSA, func(o *terraformexec.NamespaceObject) { o.SecretRefs = 1 })}, []string{"ServiceAccount/default"}},
		"default SA with imagePullSecrets":        {[]terraformexec.NamespaceObject{with(defaultSA, func(o *terraformexec.NamespaceObject) { o.ImagePullSecrets = 1 })}, []string{"ServiceAccount/default"}},
		"default SA with custom annotation":       {[]terraformexec.NamespaceObject{with(defaultSA, func(o *terraformexec.NamespaceObject) { o.AnnotationKeys = []string{"x"} })}, []string{"ServiceAccount/default"}},
		"default SA with automount override":      {[]terraformexec.NamespaceObject{with(defaultSA, func(o *terraformexec.NamespaceObject) { o.AutomountSet = true })}, []string{"ServiceAccount/default"}},
		"default SA replaced (other manager)":     {[]terraformexec.NamespaceObject{with(defaultSA, func(o *terraformexec.NamespaceObject) { o.Managers = []string{"kubectl-apply"} })}, []string{"ServiceAccount/default"}},
		"look-alike by wrong kind":                {[]terraformexec.NamespaceObject{generated(terraformexec.NamespaceObject{Kind: "Secret", Name: "kube-root-ca.crt", UID: "uid-z", DataKeys: []string{"ca.crt"}})}, []string{"Secret/kube-root-ca.crt"}},
		"object without a UID is never accounted": {[]terraformexec.NamespaceObject{{Kind: "ServiceAccount", Name: "default", Managers: []string{controllerManager}}}, []string{"ServiceAccount/default"}},
	}
	for name, tc := range cases {
		got, ours := judgeNamespace(stateNamespace{Name: "orc-ns", UID: nsUID}, terraformexec.NamespaceInspection{UID: nsUID, Objects: tc.objects}, own)
		if !ours || !reflect.DeepEqual(got, tc.foreign) {
			t.Errorf("%s: foreign %v ours %v, want %v", name, got, ours, tc.foreign)
		}
	}
	// A different namespace UID than the state recorded is never ours.
	if _, ours := judgeNamespace(stateNamespace{Name: "orc-ns", UID: nsUID}, terraformexec.NamespaceInspection{UID: "uid-other"}, own); ours {
		t.Fatal("a replaced namespace was accepted")
	}
	if _, ours := judgeNamespace(stateNamespace{Name: "orc-ns"}, terraformexec.NamespaceInspection{UID: nsUID}, own); ours {
		t.Fatal("a namespace without a recorded UID was accepted")
	}
}

func TestDefaultServiceAccountRequiresImmutableCreationBaselineWhenManagersAreAbsent(t *testing.T) {
	defaultSA := terraformexec.NamespaceObject{Kind: "ServiceAccount", Name: "default", UID: "uid-sa"}
	ns := stateNamespace{Name: "orch-v2-test-r1", UID: nsUID}
	inspection := terraformexec.NamespaceInspection{UID: nsUID, Objects: []terraformexec.NamespaceObject{defaultSA}}
	if got, ours := judgeNamespace(ns, inspection, nil); !ours || !reflect.DeepEqual(got, []string{"ServiceAccount/default"}) {
		t.Fatalf("an SA with no field-manager provenance must remain foreign without a baseline: %v ours=%v", got, ours)
	}
	baseline := &namespaceCreationBaseline{NamespaceUID: nsUID, DefaultServiceAccountUID: "uid-sa"}
	if got, ours := judgeNamespaceWithBaseline(ns, inspection, nil, baseline); !ours || len(got) != 0 {
		t.Fatalf("the exact immutable creation baseline should account for the default SA: %v ours=%v", got, ours)
	}
	for name, tc := range map[string]struct {
		inspection terraformexec.NamespaceInspection
		baseline   namespaceCreationBaseline
	}{
		"replacement UID":    {inspection: inspection, baseline: namespaceCreationBaseline{NamespaceUID: nsUID, DefaultServiceAccountUID: "uid-replaced"}},
		"replaced namespace": {inspection: terraformexec.NamespaceInspection{UID: "uid-new", Objects: []terraformexec.NamespaceObject{defaultSA}}, baseline: *baseline},
		"custom metadata":    {inspection: terraformexec.NamespaceInspection{UID: nsUID, Objects: []terraformexec.NamespaceObject{{Kind: "ServiceAccount", Name: "default", UID: "uid-sa", ImagePullSecrets: 1}}}, baseline: *baseline},
	} {
		t.Run(name, func(t *testing.T) {
			got, ours := judgeNamespaceWithBaseline(ns, tc.inspection, nil, &tc.baseline)
			wantOurs := name != "replaced namespace"
			if ours != wantOurs || !reflect.DeepEqual(got, []string{"ServiceAccount/default"}) {
				t.Fatalf("invalid baseline/object was accepted: %v ours=%v", got, ours)
			}
		})
	}
}

func TestCaptureNamespaceBaselineIsImmutableAndBoundToCreatedUIDs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		uid     string
		labels  []string
		wantErr bool
	}{
		{name: "fresh default", uid: "uid-sa"},
		{name: "custom metadata", uid: "uid-sa", labels: []string{"team"}, wantErr: true},
		{name: "empty identity", uid: "", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &baselineStore{}
			inspector := baselineInspector{inspection: terraformexec.NamespaceInspection{UID: nsUID, Objects: []terraformexec.NamespaceObject{{Kind: "ServiceAccount", Name: "default", UID: tc.uid, LabelKeys: tc.labels}}}}
			s := &Service{deps: Deps{Verification: store, Namespaces: inspector}}
			st := &execState{action: tfresource.ActionCreate, bundle: tfmodule.Bundle{Ref: tfmodule.SourceRef{ModuleID: "k8s-namespace"}}, inst: tfresource.Instance{ID: "12345678-1234-4234-8234-123456789abc"}, env: terraformexec.ProviderEnv{}}
			resources := []terraformexec.StateResource{{Type: "kubernetes_namespace_v1", ID: "orch-ns", UID: nsUID}}
			err := s.captureNamespaceBaseline(context.Background(), st, resources)
			if (err != nil) != tc.wantErr {
				t.Fatalf("capture error=%v wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			original, found, err := store.LoadVerification(context.Background(), namespaceUIDBaselineHandle(st.inst.ID, nsUID), st.inst.ID)
			if err != nil || !found {
				t.Fatalf("baseline not persisted: found=%v err=%v", found, err)
			}
			inspector.inspection.Objects[0].UID = "uid-replaced"
			if err := s.captureNamespaceBaseline(context.Background(), st, resources); err == nil {
				t.Fatal("replacement UID overwrote an established baseline")
			}
			after, found, err := store.LoadVerification(context.Background(), namespaceUIDBaselineHandle(st.inst.ID, nsUID), st.inst.ID)
			if err != nil || !found {
				t.Fatalf("baseline not readable: found=%v err=%v", found, err)
			}
			if !reflect.DeepEqual(after, original) {
				t.Fatal("immutable baseline changed")
			}
		})
	}
}

func TestCaptureNamespaceBaselineWaitsOnlyForLateDefaultServiceAccount(t *testing.T) {
	inspector := &baselineSequenceInspector{inspections: []terraformexec.NamespaceInspection{
		{UID: nsUID},
		{UID: nsUID, Objects: []terraformexec.NamespaceObject{{Kind: "ServiceAccount", Name: "default", UID: "uid-sa"}}},
	}}
	store := &baselineStore{}
	s := &Service{deps: Deps{Verification: store, Namespaces: inspector}}
	st := &execState{action: tfresource.ActionCreate, bundle: tfmodule.Bundle{Ref: tfmodule.SourceRef{ModuleID: "k8s-namespace"}}, inst: tfresource.Instance{ID: "12345678-1234-4234-8234-123456789abc"}, env: terraformexec.ProviderEnv{}}
	resources := []terraformexec.StateResource{{Type: "kubernetes_namespace_v1", ID: "orch-ns", UID: nsUID}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.captureNamespaceBaseline(ctx, st, resources); err != nil {
		t.Fatal(err)
	}
	if inspector.calls != 2 {
		t.Fatalf("inspector calls=%d, want retry only after the default account was absent", inspector.calls)
	}
}

func TestCaptureNamespaceBaselineStopsWhenReadinessExpires(t *testing.T) {
	inspector := &baselineSequenceInspector{inspections: []terraformexec.NamespaceInspection{{UID: nsUID}}}
	store := &baselineStore{}
	s := &Service{deps: Deps{Verification: store, Namespaces: inspector}}
	st := &execState{action: tfresource.ActionCreate, bundle: tfmodule.Bundle{Ref: tfmodule.SourceRef{ModuleID: "k8s-namespace"}}, inst: tfresource.Instance{ID: "12345678-1234-4234-8234-123456789abc"}, env: terraformexec.ProviderEnv{}}
	resources := []terraformexec.StateResource{{Type: "kubernetes_namespace_v1", ID: "orch-ns", UID: nsUID}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.captureNamespaceBaseline(ctx, st, resources); err == nil {
		t.Fatal("a missing default ServiceAccount outlived the readiness deadline")
	}
	if len(store.values) != 0 {
		t.Fatal("baseline was saved without a verified ServiceAccount UID")
	}
}

func TestNamespaceRecreationKeepsIndependentBaselinesAndCleanupGuards(t *testing.T) {
	ctx := context.Background()
	store := &baselineStore{}
	st := &execState{action: tfresource.ActionCreate, bundle: tfmodule.Bundle{Ref: tfmodule.SourceRef{ModuleID: "k8s-namespace"}}, inst: tfresource.Instance{ID: "12345678-1234-4234-8234-123456789abc"}}
	legacy, _ := json.Marshal(namespaceCreationBaseline{NamespaceUID: "old-ns", DefaultServiceAccountUID: "old-sa"})
	store.SaveVerification(ctx, namespaceBaselineHandle(st.inst.ID), st.inst.ID, legacy)
	inspector := baselineInspector{inspection: terraformexec.NamespaceInspection{UID: "new-ns", Objects: []terraformexec.NamespaceObject{{Kind: "ServiceAccount", Name: "default", UID: "new-sa"}}}}
	s := &Service{deps: Deps{Verification: store, Namespaces: inspector}}
	resources := []terraformexec.StateResource{{Type: "kubernetes_namespace_v1", ID: "orch-ns", UID: "new-ns"}}
	if err := s.captureNamespaceBaseline(ctx, st, resources); err != nil {
		t.Fatalf("recreating a destroyed instance reused its old baseline: %v", err)
	}
	if err := s.inspectNamespaces(ctx, st, resources); err != nil {
		t.Fatalf("the recreated namespace cannot be cleaned: %v", err)
	}
	old, _, _ := store.LoadVerification(ctx, namespaceBaselineHandle(st.inst.ID), st.inst.ID)
	if !reflect.DeepEqual(old, legacy) {
		t.Fatal("recreation overwrote historical provenance")
	}
	inspector.inspection.Objects[0].UID = "replacement-sa"
	s.deps.Namespaces = inspector
	if err := s.captureNamespaceBaseline(ctx, st, resources); err == nil {
		t.Fatal("a replaced ServiceAccount overwrote the current baseline")
	}
	if err := s.inspectNamespaces(ctx, st, resources); !errors.Is(err, ErrNamespaceContents) {
		t.Fatalf("a replaced ServiceAccount was allowed through cleanup: %v", err)
	}
	// Missing proof for an already-running namespace is not repaired by
	// inspection or by the old incarnation's proof.
	delete(store.values, namespaceUIDBaselineHandle(st.inst.ID, "new-ns"))
	if err := s.inspectNamespaces(ctx, st, resources); !errors.Is(err, ErrNamespaceContents) {
		t.Fatalf("cleanup inferred ownership without a matching proof: %v", err)
	}
}

func TestNamespaceCleanupReadsLegacyBaselineButNeverFallsBackFromCorruption(t *testing.T) {
	ctx := context.Background()
	store := &baselineStore{}
	st := &execState{inst: tfresource.Instance{ID: "12345678-1234-4234-8234-123456789abc"}}
	legacy, _ := json.Marshal(namespaceCreationBaseline{NamespaceUID: nsUID, DefaultServiceAccountUID: "sa"})
	store.SaveVerification(ctx, namespaceBaselineHandle(st.inst.ID), st.inst.ID, legacy)
	s := &Service{deps: Deps{Verification: store, Namespaces: baselineInspector{inspection: terraformexec.NamespaceInspection{UID: nsUID, Objects: []terraformexec.NamespaceObject{{Kind: "ServiceAccount", Name: "default", UID: "sa"}}}}}}
	resources := []terraformexec.StateResource{{Type: "kubernetes_namespace_v1", ID: "orch-ns", UID: nsUID}}
	if err := s.inspectNamespaces(ctx, st, resources); err != nil {
		t.Fatalf("a valid historical baseline stopped working: %v", err)
	}
	store.SaveVerification(ctx, namespaceUIDBaselineHandle(st.inst.ID, nsUID), st.inst.ID, []byte("invalid"))
	if err := s.inspectNamespaces(ctx, st, resources); !errors.Is(err, ErrNamespaceContents) {
		t.Fatalf("corrupt new proof fell back to a legacy proof: %v", err)
	}
}
