package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	catalogapp "orchestrator/internal/application/catalog"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

func TestApplicationPurgeRemovesHeldHistoryAndReleasesCatalog(t *testing.T) {
	f := newStateFixture(t)
	ctx := context.Background()
	preview := ApplicationPurge{Organization: "acme", Application: "app"}
	execute := preview
	execute.ConfirmationName, execute.WritersStopped = "App", true

	other := application.Application{ID: ids.New(), Key: "other", OrganizationKey: "acme", Name: "Other", Version: 1}
	if err := f.console.SaveApplication(ctx, other); err != nil {
		t.Fatal(err)
	}
	otherEnv := environment.Environment{
		ID: ids.New(), ApplicationID: other.ID, ApplicationKey: other.Key,
		Key: "staging", Name: "staging", Type: "staging", NamespaceIdentity: "ns-other", Version: 1,
	}
	if err := f.console.SaveEnvironment(ctx, otherEnv); err != nil {
		t.Fatal(err)
	}

	const typeKey, defKey = "purge-test", "purge-test-def"
	if err := f.console.CreateResourceType(ctx, "acme", resource.Type{Key: typeKey}); err != nil {
		t.Fatal(err)
	}
	if err := f.console.CreateResourceDefinition(ctx, "acme", resource.Definition{
		Key: defKey, Name: "Test Definition", Version: 1, ResourceTypeKey: typeKey,
		DriverType: resource.DriverKubernetes, Criteria: []resource.Criterion{{}},
	}); err != nil {
		t.Fatal(err)
	}
	active, err := f.console.UpsertActiveResource(ctx, resource.ActiveResource{
		OrganizationKey: "acme", Descriptor: resource.Descriptor{Type: typeKey, Class: "default", ID: "shared.data"},
		Scope: resource.Scope{Type: resource.ScopeShared, ID: "app.staging~g2"}, DefinitionKey: defKey,
		Status: resource.StatusReady, Outputs: map[string]any{}, ExecutorState: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	const sharedDef = "purge-shared-def"
	if err := f.console.CreateResourceType(ctx, "acme", resource.Type{Key: "purge-shared"}); err != nil {
		t.Fatal(err)
	}
	if err := f.console.CreateResourceDefinition(ctx, "acme", resource.Definition{
		Key: sharedDef, Name: "Shared Test Definition", Version: 1, ResourceTypeKey: "purge-shared",
		DriverType: resource.DriverKubernetes, Criteria: []resource.Criterion{{}},
	}); err != nil {
		t.Fatal(err)
	}
	shared, err := f.console.UpsertActiveResource(ctx, resource.ActiveResource{
		OrganizationKey: "acme", Descriptor: resource.Descriptor{Type: "purge-shared", Class: "default", ID: "shared.data"},
		Scope: resource.Scope{Type: resource.ScopeShared, ID: "app.staging"}, DefinitionKey: sharedDef,
		Status: resource.StatusReady, Outputs: map[string]any{}, ExecutorState: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// The skipped infrastructure calls are represented by protected state,
	// a retained operation, and a failed but receipted Fleet publication.
	orgID, envID, opID := f.inst.Identity.OrganizationID, f.inst.Identity.EnvironmentID, f.op.ID
	unitID, artifactID, revisionID, deploymentID := ids.New(), ids.New(), ids.New(), ids.New()
	otherDeployment := ids.New()
	sqlOn(t, f.db.Admin,
		`UPDATE applications SET status='DELETING',deletion='{"status":"FAILED"}' WHERE application_key='app'`,
		`UPDATE environment_operations SET status='INTERRUPTED' WHERE id='`+opID+`'`,
		`INSERT INTO delivery_units(id,organization_id,environment_id,target_generation,delivery_epoch,unit_key,workload_id,phase,target_ref,version,created_at,updated_at)
 VALUES('`+unitID+`','`+orgID+`','`+envID+`',0,0,'migrate','_environment','JOB','{}',1,now(),now())`,
		`INSERT INTO reconciliation_private_artifacts(id,organization_id,owner_id,purpose,key_id,nonce,ciphertext,aad_version,created_at)
 VALUES('`+artifactID+`','`+orgID+`','`+unitID+`','DELIVERY_PAYLOAD','test-key',decode(repeat('00',12),'hex'),convert_to('private-ciphertext','UTF8'),1,now())`,
		`INSERT INTO delivery_revisions(id,unit_id,organization_id,environment_operation_id,artifact_id,payload_digest,selection,created_at)
 VALUES('`+revisionID+`','`+unitID+`','`+orgID+`','`+opID+`','`+artifactID+`',repeat('a',64),'{}',now())`,
		`UPDATE delivery_units SET pending_revision_id='`+revisionID+`' WHERE id='`+unitID+`'`,
		`INSERT INTO delivery_intents(id,unit_id,revision_id,environment_operation_id,action,status,request_id,request_digest,owner,fence,version,receipt,created_at,updated_at)
 VALUES(gen_random_uuid(),'`+unitID+`','`+revisionID+`','`+opID+`','SUBMIT','FAILED','request',repeat('a',64),'prior-writer',1,1,
 '{"Handle":{"gitrepo":"test-repo","release":"test-release","namespace":"ns-app-staging","private":"private-receipt"}}',now(),now())`,
		`INSERT INTO deployments(id,environment_id,organization_key,application_key,environment_key,action,workload_id,actor_ref,status,base_environment_version,started_at)
 VALUES('`+deploymentID+`','`+envID+`','acme','app','staging','DEPLOY','web','tester','SUCCEEDED',1,now());
 INSERT INTO deployment_delta_snapshots(id,deployment_id,document,document_hash,metadata,created_at)
 VALUES(gen_random_uuid(),'`+deploymentID+`','{}','test-hash','{}',now())`,
		`INSERT INTO deployments(id,environment_id,organization_key,application_key,environment_key,action,workload_id,actor_ref,status,base_environment_version,started_at)
 VALUES('`+otherDeployment+`','`+otherEnv.ID+`','acme','other','staging','DEPLOY','web','tester','FAILED',1,now());
 INSERT INTO deployment_resources(deployment_id,node_descriptor,active_resource_id,status,resolved_inputs,output_snapshot,batch_index,started_at)
 VALUES('`+otherDeployment+`','purge-shared.default#shared.data','`+shared.ID+`','READY','{}','{}',0,now())`,
	)

	if err := catalogapp.NewService(f.console).DeleteEntry(ctx, "acme", defKey, true); err == nil {
		t.Fatal("definition deletion should be blocked before purge")
	}
	result, err := PurgeApplication(ctx, f.db.Owner, preview)
	if err != nil || result.Purged || result.Status != "DELETING" || result.InstallationID != f.console.Identity().Installation ||
		len(result.Environments) != 1 || result.Environments[0].Namespace != "ns-app-staging" ||
		len(result.Fleet) != 1 || result.Fleet[0].GitRepo != "test-repo" || len(result.Resources) != 1 {
		t.Fatalf("preview = %+v, %v", result, err)
	}
	if wire, _ := json.Marshal(result); strings.Contains(string(wire), "private-") {
		t.Fatalf("preview exposed private data: %s", wire)
	}
	if _, err := PurgeApplication(ctx, f.db.Console, preview); err == nil || !strings.Contains(err.Error(), "schema owner") {
		t.Fatalf("runtime login accepted: %v", err)
	}
	if _, err := PurgeApplication(ctx, f.db.Owner, execute); err == nil || !strings.Contains(err.Error(), "writer connections") {
		t.Fatalf("connected writer did not block purge: %v", err)
	}
	f.console.Close()
	if _, err := PurgeApplication(ctx, f.db.Owner, execute); err == nil || !strings.Contains(err.Error(), "writer connections") {
		t.Fatalf("connected State writer did not block purge: %v", err)
	}
	f.state.Close()

	wrong := execute
	wrong.ConfirmationName = "Other"
	if _, err := PurgeApplication(ctx, f.db.Owner, wrong); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("wrong confirmation accepted: %v", err)
	}
	wrong = execute
	wrong.Organization = "another-org"
	if _, err := PurgeApplication(ctx, f.db.Owner, wrong); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("another Organization accepted: %v", err)
	}

	// Inject a cross-app FK to cause an error late in the purge. Earlier
	// deletes and trigger suspensions must roll back together.
	conflict := ids.New()
	sqlOn(t, f.db.Admin, `INSERT INTO workload_instances(id,environment_id,workload_id,last_deployment_id,target_ref,manifest_digest,status,observed_at)
 VALUES('`+conflict+`','`+otherEnv.ID+`','cross-app','`+deploymentID+`','{}','digest','READY',now())`)
	if _, err := PurgeApplication(ctx, f.db.Owner, execute); err == nil || !strings.Contains(err.Error(), "23503") {
		t.Fatalf("cross-app dependency did not force rollback: %v", err)
	}
	stillThere, err := PurgeApplication(ctx, f.db.Owner, preview)
	if err != nil || stillThere.ApplicationID != result.ApplicationID || len(stillThere.Fleet) != 1 || len(stillThere.Resources) != 1 {
		t.Fatalf("late failure left partial deletion or changed schema guards: %+v %v", stillThere, err)
	}
	sqlOn(t, f.db.Admin, `DELETE FROM workload_instances WHERE id='`+conflict+`'`)

	result, err = PurgeApplication(ctx, f.db.Owner, execute)
	if err != nil || !result.Purged {
		t.Fatalf("purge = %+v %v", result, err)
	}
	if _, err := PurgeApplication(ctx, f.db.Owner, execute); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("purge repeated accepted: %v", err)
	}

	console, err := Open(ctx, f.db.Console)
	if err != nil {
		t.Fatalf("guard or runtime grants changed: %v", err)
	}
	t.Cleanup(console.Close)
	if _, err := console.GetApplication(ctx, "app"); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("Application was not hard-deleted: %v", err)
	}
	if remaining, err := console.ListActiveResources(ctx, "acme"); err != nil || len(remaining) != 1 || remaining[0].ID != shared.ID {
		t.Fatalf("expected only shared resource %s to remain after removing %s: %+v %v", shared.ID, active.ID, remaining, err)
	}
	if actual, err := console.GetEnvironment(ctx, other.Key, otherEnv.Key); err != nil || actual.ID != otherEnv.ID {
		t.Fatalf("another Application's Environment changed: %+v %v", actual, err)
	}
	catalog := catalogapp.NewService(console)
	if err := catalog.DeleteEntry(ctx, "acme", sharedDef, true); err == nil {
		t.Fatal("shared definition should still be protected")
	}
	if err := catalog.DeleteEntry(ctx, "acme", defKey, true); err != nil {
		t.Fatalf("definition still blocked after purge: %v", err)
	}
	if err := catalog.DeleteEntry(ctx, "acme", typeKey, false); err != nil {
		t.Fatalf("type still blocked after purge: %v", err)
	}
}
