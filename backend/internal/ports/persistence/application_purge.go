package persistence

import "context"

type ApplicationPurgeReport struct {
	InstallationID    string                  `json:"installationId"`
	ApplicationID     string                  `json:"applicationId"`
	Organization      string                  `json:"organization"`
	Application       string                  `json:"application"`
	DisplayName       string                  `json:"displayName"`
	Status            string                  `json:"status"`
	Busy              bool                    `json:"busy"`
	PriorStopRequired bool                    `json:"priorStopRequired"`
	Purged            bool                    `json:"purged"`
	Environments      []ApplicationPurgeEnv   `json:"environments"`
	Resources         []string                `json:"resources"`
	Fleet             []ApplicationPurgeFleet `json:"fleet"`
	GitRepos          []string                `json:"gitRepositories"`
}

type ApplicationPurgeEnv struct {
	Key       string `json:"key"`
	Namespace string `json:"namespace"`
}

type ApplicationPurgeFleet struct {
	GitRepo   string `json:"gitRepo"`
	Release   string `json:"release"`
	Namespace string `json:"namespace"`
}

type WebApplicationPurgeStore interface {
	WebApplicationPurgeEnabled(context.Context) (bool, error)
	PreviewWebApplicationPurge(ctx context.Context, organization, application string) (ApplicationPurgeReport, error)
	ExecuteWebApplicationPurge(ctx context.Context, organization, application, expectedID, name string, priorStopped bool) (string, error)
}
