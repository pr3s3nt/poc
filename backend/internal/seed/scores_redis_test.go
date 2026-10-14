package seed_test

import (
	"testing"

	"orchestrator/internal/seed"
)

func TestAcceptanceScoresRedisWiresOneSharedCacheToBackendAndWorker(t *testing.T) {
	docs := seed.AcceptanceScoresRedis(seed.Defaults(), "acceptance-cache", "in-cluster", "acceptance", "dev", "run-1")
	for _, name := range []string{"backend", "worker"} {
		vars := docs[name]["containers"].(map[string]any)["main"].(map[string]any)["variables"].(map[string]any)
		if vars["REDIS_PASSWORD"] != "${resources.cache.password}" || vars["ACCEPTANCE_CACHE_RUN"] != "run-1" || vars["ACCEPTANCE_CACHE_TTL_SECONDS"] != "20" {
			t.Errorf("%s variables %v", name, vars)
		}
		cache := docs[name]["resources"].(map[string]any)["cache"].(map[string]any)
		if cache["type"] != "redis" || cache["id"] != "acceptance-cache" || cache["class"] != "in-cluster" {
			t.Errorf("%s cache %v", name, cache)
		}
	}
	if _, ok := docs["frontend"]["resources"]; ok {
		t.Error("frontend must not receive Redis")
	}
	// The plain documents stay untouched (no shared mutation).
	plain := seed.AcceptanceScores(seed.Defaults())
	if _, ok := plain["backend"]["resources"].(map[string]any)["cache"]; ok {
		t.Error("legacy documents gained a cache")
	}
}
