package deployment_test

import (
	"orchestrator/internal/renderengine"
	"os"
	"testing"
)

func TestMain(m *testing.M) { renderengine.RunWorkerIfRequested(); os.Exit(m.Run()) }
