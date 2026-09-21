package seed

// AcceptanceScores returns the three Score documents of the acceptance
// application. frontend, backend and worker are independent workloads; backend
// and worker share one stable database resource ID and pass the same `params`.
func AcceptanceScores(o Options) map[string]map[string]any {
	database := o.Database
	if database == "" {
		database = "acceptance"
	}
	username := o.DatabaseUser
	if username == "" {
		username = "app"
	}
	dbRef := map[string]any{
		"type":   "postgres",
		"class":  "default",
		"id":     o.SharedDatabaseID,
		"params": map[string]any{"database": database, "username": username},
	}
	dbVariables := map[string]any{
		"PGHOST":     "${resources.db.host}",
		"PGPORT":     "${resources.db.port}",
		"PGDATABASE": "${resources.db.database}",
		"PGUSER":     "${resources.db.username}",
		"PGPASSWORD": "${resources.db.password}",
	}

	backendVariables := map[string]any{"PORT": "8080"}
	workerVariables := map[string]any{"POLL_INTERVAL_MS": "500"}
	for k, v := range dbVariables {
		backendVariables[k] = v
		workerVariables[k] = v
	}

	return map[string]map[string]any{
		"backend": {
			"apiVersion": "score.dev/v1b1",
			"metadata":   map[string]any{"name": "backend"},
			"containers": map[string]any{
				"main": map[string]any{
					"image":          o.Images.Backend,
					"variables":      backendVariables,
					"readinessProbe": map[string]any{"path": "/readyz", "port": 8080},
					"livenessProbe":  map[string]any{"path": "/healthz", "port": 8080},
				},
			},
			"service":   map[string]any{"ports": map[string]any{"http": map[string]any{"port": 8080, "targetPort": 8080}}},
			"resources": map[string]any{"db": dbRef},
		},
		"worker": {
			"apiVersion": "score.dev/v1b1",
			"metadata":   map[string]any{"name": "worker"},
			"containers": map[string]any{
				"main": map[string]any{"image": o.Images.Worker, "variables": workerVariables},
			},
			"resources": map[string]any{"db": dbRef},
		},
		"frontend": {
			"apiVersion": "score.dev/v1b1",
			"metadata":   map[string]any{"name": "frontend"},
			"containers": map[string]any{
				"main": map[string]any{
					"image": o.Images.Frontend,
					"variables": map[string]any{
						"PORT":        "8080",
						"BACKEND_URL": "http://backend:8080",
					},
					"readinessProbe": map[string]any{"path": "/healthz", "port": 8080},
					"livenessProbe":  map[string]any{"path": "/healthz", "port": 8080},
				},
			},
			"service": map[string]any{"ports": map[string]any{"http": map[string]any{"port": 8080, "targetPort": 8080}}},
		},
	}
}

// AcceptanceOrder is the deployment order of the acceptance workloads.
// The database is provisioned with the first workload that declares it.
func AcceptanceOrder() []string { return []string{"backend", "worker", "frontend"} }
