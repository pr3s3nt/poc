// Package application implements the self-service UC-01 creation transaction.
package application

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	appdomain "orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

var subdomainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

type CreateCommand struct{ OrganizationKey, Name, Subdomain, BaseDomain string }
type Result struct {
	Application  appdomain.Application
	Environments []environment.Environment
}
type Service struct{ store persistence.Store }

func NewService(store persistence.Store) *Service { return &Service{store: store} }
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (Result, error) {
	cmd.Name = strings.TrimSpace(cmd.Name)
	cmd.Subdomain = strings.ToLower(strings.TrimSpace(cmd.Subdomain))
	if cmd.Name == "" || !subdomainPattern.MatchString(cmd.Subdomain) {
		return Result{}, fmt.Errorf("application: name and valid subdomain are required")
	}
	var result Result
	err := s.store.Transact(ctx, func(ctx context.Context) error {
		org, err := s.store.GetOrganization(ctx, cmd.OrganizationKey)
		if err != nil {
			return err
		}
		conn, err := s.store.GetConnection(ctx, org.DefaultConnectionKey)
		if err != nil {
			return err
		}
		if conn.Status != appdomain.ConnectionReady {
			return fmt.Errorf("application: default connection is not ready")
		}
		apps, err := s.store.ListApplications(ctx)
		if err != nil {
			return err
		}
		for _, existing := range apps {
			if existing.Subdomain == cmd.Subdomain || (existing.OrganizationKey == cmd.OrganizationKey && strings.EqualFold(existing.Name, cmd.Name)) {
				return fmt.Errorf("application: name or subdomain already exists")
			}
		}
		profile, region, status := appdomain.ProfileInternalK8s, "", appdomain.RuntimeReady
		if conn.Kind == appdomain.ConnectionAWS {
			profile, region, status = appdomain.ProfileAWSEKS, conn.ConfigString("region"), appdomain.RuntimePending
			if region == "" {
				return fmt.Errorf("application: default AWS connection needs a region")
			}
		}
		app := appdomain.Application{Key: ids.New(), OrganizationKey: cmd.OrganizationKey, Name: cmd.Name, Subdomain: cmd.Subdomain, Profile: profile, ConnectionKey: conn.Key, Region: region, RuntimeStatus: status, ConfigurationProvider: "vault"}
		if err := app.Validate(); err != nil {
			return err
		}
		if err := s.store.SaveApplication(ctx, app); err != nil {
			return err
		}
		for _, key := range []string{"staging", "production"} {
			set := environment.DeploymentSet{ID: ids.New(), EnvironmentKey: app.Key + "/" + key, Document: environment.NewDocument(), DocumentHash: "empty", CreatedAt: time.Now().UTC()}
			env := environment.Environment{Key: key, ApplicationKey: app.Key, Name: strings.Title(key), Type: key, NamespaceIdentity: "app-" + app.Key + "-" + key, CurrentDeploymentSetID: set.ID}
			if err := s.store.SaveDeploymentSet(ctx, set); err != nil {
				return err
			}
			if err := s.store.SaveEnvironment(ctx, env); err != nil {
				return err
			}
			result.Environments = append(result.Environments, env)
		}
		result.Application = app
		return nil
	})
	return result, err
}
