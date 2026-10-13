package terraform

import (
	"io/fs"
	"testing"
)

func TestModulesAreEmbedded(t *testing.T) {
	for _, module := range []string{"vpc", "eks", "aurora"} {
		if _, err := fs.Stat(PackagedModules(), module+"/main.tf"); err != nil {
			t.Errorf("module %s is not embedded: %v", module, err)
		}
	}
}
