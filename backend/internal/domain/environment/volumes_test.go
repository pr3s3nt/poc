package environment

import "testing"

func TestContainerRoleValidation(t *testing.T) {
	base := func() map[string]Container {
		return map[string]Container{"app": {Image: "app"}, "prepare": {Image: "init", Role: "init", InitOrder: 2}, "agent": {Image: "agent", Role: "sidecar", InitOrder: 1}}
	}
	if err := ValidateContainerRoles(base()); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(map[string]Container){
		func(c map[string]Container) { delete(c, "app") },
		func(c map[string]Container) { v := c["prepare"]; v.InitOrder = 1; c["prepare"] = v },
		func(c map[string]Container) {
			v := c["prepare"]
			v.ReadinessProbe = &Probe{Path: "/ready", Port: 80}
			c["prepare"] = v
		},
		func(c map[string]Container) { v := c["agent"]; v.Role = "other"; c["agent"] = v },
	} {
		c := base()
		change(c)
		if ValidateContainerRoles(c) == nil {
			t.Fatal("accepted invalid role/order")
		}
	}
}
func TestLocalVolumesValidationAndClone(t *testing.T) {
	base := func() *WorkloadExecution {
		return &WorkloadExecution{Version: 1, Kind: "Deployment", Volumes: []PodVolume{{Name: "shared", EmptyDir: &EmptyDirVolume{Medium: "Memory", SizeLimit: "128Mi"}}, {Name: "config", ConfigMap: &ConfigMapVolume{Files: []ConfigFile{{Name: "app.conf", Value: "keep\n${literal}"}}}}}, VolumeMounts: []VolumeMount{{Volume: "shared", Container: "app", Path: "/shared"}, {Volume: "config", Container: "app", Path: "/config/app.conf", ReadOnly: true, SubPath: "app.conf"}}}
	}
	containers := map[string]Container{"app": {Image: "app"}}
	if err := base().Validate(containers, 1, false, nil); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*WorkloadExecution){
		func(e *WorkloadExecution) { e.Volumes[0].EmptyDir.SizeLimit = "0Mi" },
		func(e *WorkloadExecution) { e.Volumes[0].EmptyDir.SizeLimit = "garbage" },
		func(e *WorkloadExecution) { e.Volumes[0].Name = "config" },
		func(e *WorkloadExecution) { e.VolumeMounts[1].ReadOnly = false },
		func(e *WorkloadExecution) { e.VolumeMounts[1].SubPath = "../app.conf" },
		func(e *WorkloadExecution) { e.VolumeMounts[1].SubPath = "missing" },
		func(e *WorkloadExecution) { e.VolumeMounts[1].Container = "missing" },
		func(e *WorkloadExecution) { e.VolumeMounts[1].Path = "/shared/nested" },
		func(e *WorkloadExecution) { e.VolumeMounts[0].Path = "/var/run" },
		func(e *WorkloadExecution) {
			e.Volumes[1].ConfigMap.Files = append(e.Volumes[1].ConfigMap.Files, ConfigFile{Name: "app.conf"})
		},
		func(e *WorkloadExecution) { e.Template = TemplatePin{ID: "partial"} },
		func(e *WorkloadExecution) { e.Inputs = map[string]any{"template-only": "input"} },
	} {
		e := base()
		change(e)
		if e.Validate(containers, 1, false, nil) == nil {
			t.Fatal("accepted invalid volume intent")
		}
	}
	e := base()
	cloned := e.Clone()
	cloned.Volumes[1].ConfigMap.Files[0].Value = "changed"
	cloned.VolumeMounts[0].Path = "/else"
	cloned.Volumes[0].EmptyDir.Medium = ""
	if e.Volumes[1].ConfigMap.Files[0].Value == "changed" || e.VolumeMounts[0].Path != "/shared" || e.Volumes[0].EmptyDir.Medium != "Memory" {
		t.Fatal("clone aliases source")
	}
}

func TestProbeActionAndTimingValidation(t *testing.T) {
	zero, one, two := 0, 1, 2
	for _, p := range []*Probe{{Path: "/health", Port: 8080, InitialDelaySeconds: &zero}, {Type: "tcp", Port: 5432, TimeoutSeconds: &two}, {Type: "exec", Command: []string{"sh", "-c", "check"}, SuccessThreshold: &one}} {
		if err := p.Validate(true); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []*Probe{{Type: "exec"}, {Type: "tcp", Port: 80, Path: "/http"}, {Type: "http", Path: "/", Port: 80, Command: []string{"cmd"}}, {Path: "/", Port: 80, PeriodSeconds: &zero}, {Path: "/", Port: 80, SuccessThreshold: &two}} {
		if p.Validate(true) == nil {
			t.Fatal("accepted invalid probe")
		}
	}
}

func TestHostPathVolumeValidationAndClone(t *testing.T) {
	containers := map[string]Container{"app": {Image: "app"}}
	base := func() *WorkloadExecution {
		return &WorkloadExecution{Version: 1, Kind: "Deployment", Volumes: []PodVolume{{Name: "node-data", HostPath: &HostPathVolume{Path: "/var/local/app", Type: "Directory"}}}, VolumeMounts: []VolumeMount{{Volume: "node-data", Container: "app", Path: "/data", ReadOnly: true}}}
	}
	for _, typ := range []string{"", "Directory", "DirectoryOrCreate", "File", "FileOrCreate", "Socket", "CharDevice", "BlockDevice"} {
		e := base()
		e.Volumes[0].HostPath.Type = typ
		if err := e.Validate(containers, 1, false, nil); err != nil {
			t.Fatalf("type %q: %v", typ, err)
		}
	}
	for _, sourcePath := range []string{"", "relative", "/a/../b", "/a//b", "/a/", "/a\\b", "/a\nb", "/a\x00b"} {
		e := base()
		e.Volumes[0].HostPath.Path = sourcePath
		if e.Validate(containers, 1, false, nil) == nil {
			t.Fatalf("accepted invalid host path %q", sourcePath)
		}
	}
	e := base()
	e.Volumes[0].HostPath.Type = "Unknown"
	if e.Validate(containers, 1, false, nil) == nil {
		t.Fatal("accepted unknown host type")
	}
	e = base()
	e.Volumes[0].EmptyDir = &EmptyDirVolume{}
	if e.Validate(containers, 1, false, nil) == nil {
		t.Fatal("accepted multiple sources")
	}
	e = base()
	e.VolumeMounts[0].ReadOnly = false
	if err := e.Validate(containers, 1, false, nil); err != nil {
		t.Fatal("explicit writable mount rejected:", err)
	}
	cloned := e.Clone()
	cloned.Volumes[0].HostPath.Path = "/changed"
	if e.Volumes[0].HostPath.Path != "/var/local/app" {
		t.Fatal("clone aliases host path")
	}
}
