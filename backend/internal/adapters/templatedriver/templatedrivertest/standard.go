// Package templatedrivertest builds the real template driver with immutable in-memory source content.
package templatedrivertest

import (
	"context"
	"orchestrator/internal/adapters/templatedriver"
	"orchestrator/internal/adapters/templatesources"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/templatesource"
	"orchestrator/internal/renderengine"
	standardtemplates "orchestrator/template-sources"
	"strings"
	"testing"
)

type contentStore struct {
	content renderengine.Bundle
	digest  string
}

func (s contentStore) Load(_ context.Context, pin resource.TemplateSource) (renderengine.Bundle, error) {
	if pin.Digest != s.digest {
		return renderengine.Bundle{}, templatedriver.ErrBundleUnavailable
	}
	return s.content, nil
}
func Standard(t testing.TB) (*templatedriver.Renderer, resource.RenderingSelection) {
	t.Helper()
	files := map[string][]byte{}
	for name, content := range standardtemplates.Files() {
		files[name] = []byte(content)
	}
	b, err := renderengine.Validate(files)
	if err != nil {
		t.Fatal(err)
	}
	digest := templatesources.Digest(files)
	r := templatedriver.New()
	r.SetGitBundles(contentStore{b, digest}, renderengine.Executor{})
	record := templatesource.Bundle{ID: standardtemplates.ID("test"), Source: templatesource.Source{Kind: "inline", URL: "inline:sha256:" + digest, Path: ".", Revision: strings.Repeat("a", 64), ContentSHA256: digest}, EngineContract: templatesource.EngineContract, RendererVersion: templatesource.RendererVersion, RendererBinary: r.BinaryDigest(), Contract: b.Contract}
	inputs, err := templatesource.Effective(b.Contract.InputsSchema, nil)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := templatesource.InputsHash(inputs)
	return r, resource.RenderingSelection{DefinitionKey: "render", DefinitionHash: "pin", DriverType: resource.DriverTemplate, Bundle: resource.GitRenderPin(record), Inputs: inputs, InputsHash: hash}
}
