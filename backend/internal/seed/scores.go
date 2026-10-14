package seed

// AcceptanceScores returns the three Score documents of the acceptance
// application. frontend, backend and worker are independent workloads; backend
// and worker share one stable database resource ID and pass the same `params`.
// Container resources cover the three renderer cases of UC-06 BR-11: backend
// declares full requests/limits, worker declares a partial set and frontend
// declares none.
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
					"resources": map[string]any{
						"requests": map[string]any{"cpu": "50m", "memory": "64Mi"},
						"limits":   map[string]any{"cpu": "500m", "memory": "256Mi"},
					},
				},
			},
			"service":   map[string]any{"ports": map[string]any{"http": map[string]any{"port": 8080, "targetPort": 8080}}},
			"resources": map[string]any{"db": dbRef},
		},
		"worker": {
			"apiVersion": "score.dev/v1b1",
			"metadata":   map[string]any{"name": "worker"},
			"containers": map[string]any{
				"main": map[string]any{
					"image":     o.Images.Worker,
					"variables": workerVariables,
					"resources": map[string]any{
						"requests": map[string]any{"memory": "48Mi"},
						"limits":   map[string]any{"memory": "128Mi"},
					},
				},
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

// CacheTTLSeconds is the acceptance cache TTL: short enough to wait out in a
// recording, long enough for the worker write and the backend hit assertions.
const CacheTTLSeconds = 20

// AcceptanceScoresRedis returns the three acceptance Score documents with one
// shared Redis (resource ID cacheID, class cacheClass) wired to the backend
// and the worker (UC-05/UC-06, I04-09 contract). The Redis password reaches
// the Pods only through the secret output binding; the cache scope variables
// carry the Application, Environment and run so keys never collide.
func AcceptanceScoresRedis(o Options, cacheID, cacheClass, application, environment, runID string) map[string]map[string]any {
	docs := AcceptanceScores(o)
	cacheRef := map[string]any{"type": "redis", "class": cacheClass, "id": cacheID}
	vars := map[string]any{
		"REDIS_HOST":                   "${resources.cache.host}",
		"REDIS_PORT":                   "${resources.cache.port}",
		"REDIS_USERNAME":               "${resources.cache.username}",
		"REDIS_PASSWORD":               "${resources.cache.password}",
		"REDIS_TLS":                    "${resources.cache.tls}",
		"ACCEPTANCE_CACHE_APP":         application,
		"ACCEPTANCE_CACHE_ENV":         environment,
		"ACCEPTANCE_CACHE_RUN":         runID,
		"ACCEPTANCE_CACHE_TTL_SECONDS": itoa(CacheTTLSeconds),
	}
	for _, name := range []string{"backend", "worker"} {
		doc := docs[name]
		main := doc["containers"].(map[string]any)["main"].(map[string]any)
		variables := main["variables"].(map[string]any)
		for k, v := range vars {
			variables[k] = v
		}
		doc["resources"].(map[string]any)["cache"] = cloneMap(cacheRef)
	}
	return docs
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for ; n > 0; n /= 10 {
		digits = string(rune('0'+n%10)) + digits
	}
	return digits
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
