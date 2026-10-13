package tfexec

import (
	"reflect"
	"testing"

	"orchestrator/internal/ports/terraformexec"
)

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
