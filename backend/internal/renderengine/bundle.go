package renderengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"

	"orchestrator/internal/domain/templatesource"
)

// Bundle is one validated bundle tree: the contract plus the file contents
// the stages read. It never holds a host path.
type Bundle struct {
	Contract templatesource.Contract
	Files    map[string][]byte
}

// File returns the content of a bundle-relative file.
func (b Bundle) File(name string) []byte { return b.Files[name] }

// Validate checks a complete selected tree against version 1: limits,
// manifest, schema, defaults, template syntax and helper names, and
// provisioner restrictions. It executes no author template.
func Validate(files map[string][]byte) (Bundle, error) {
	if len(files) > templatesource.MaxFiles {
		return Bundle{}, invalid("", "the bundle has more than %d files", templatesource.MaxFiles)
	}
	total := 0
	for name, content := range files {
		if !templatesource.ValidFilePath(name) {
			return Bundle{}, invalid("", "a file path is not a clean relative path")
		}
		if len(content) > templatesource.MaxFileBytes {
			return Bundle{}, invalid(name, "the file is larger than 1 MiB")
		}
		total += len(content)
	}
	if total > templatesource.MaxTreeBytes {
		return Bundle{}, invalid("", "the bundle is larger than 4 MiB")
	}
	raw, ok := files[templatesource.ManifestFile]
	if !ok {
		return Bundle{}, invalid(templatesource.ManifestFile, "the manifest is missing")
	}
	manifest, err := templatesource.ParseManifest(raw)
	if err != nil {
		return Bundle{}, err
	}
	need := func(name string) ([]byte, error) {
		content, ok := files[name]
		if !ok {
			return nil, invalid(name, "the file listed in bundle.yaml is missing")
		}
		return content, nil
	}
	schemaRaw, err := need(manifest.InputsSchema)
	if err != nil {
		return Bundle{}, err
	}
	schema, err := strictJSONObject(schemaRaw)
	if err != nil {
		return Bundle{}, invalid(manifest.InputsSchema, "the inputs schema is not one JSON object without duplicate keys")
	}
	if err := templatesource.ValidateSchema(schema); err != nil {
		var invalidErr *templatesource.InvalidError
		if errors.As(err, &invalidErr) {
			invalidErr.File = manifest.InputsSchema
		}
		return Bundle{}, err
	}
	defaultsRaw, err := need(manifest.Defaults)
	if err != nil {
		return Bundle{}, err
	}
	defaults, err := yamlObject(defaultsRaw)
	if err != nil {
		return Bundle{}, invalid(manifest.Defaults, "the defaults are not one YAML object without duplicate keys")
	}
	templates := append(append([]string{}, manifest.Resources...), manifest.Patches...)
	if manifest.Normalize != "" {
		templates = append(templates, manifest.Normalize)
	}
	for _, name := range templates {
		source, err := need(name)
		if err != nil {
			return Bundle{}, err
		}
		if _, err := Parse(name, string(source)); err != nil {
			return Bundle{}, invalid(name, "the template does not parse with the template-v1 helpers (line %d)", line(err))
		}
	}
	return Bundle{Contract: templatesource.Contract{Manifest: manifest, InputsSchema: schema, Defaults: defaults, InputContract: manifest.InputContract}, Files: files}, nil
}

func invalid(file, format string, args ...any) error {
	return &templatesource.InvalidError{File: file, Reason: fmt.Sprintf(format, args...)}
}

// strictJSONObject decodes one JSON object and rejects duplicate keys at any
// depth and trailing data.
func strictJSONObject(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := checkDuplicates(decoder); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data")
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return nil, errors.New("not an object")
	}
	return out, nil
}

func checkDuplicates(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, _ := key.(string)
			if seen[name] {
				return errors.New("duplicate key")
			}
			seen[name] = true
			if err := checkDuplicates(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := checkDuplicates(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	return errors.New("unexpected delimiter")
}

// yamlObject decodes one YAML mapping (yaml.v3 rejects duplicate keys) into a
// JSON-compatible tree.
func yamlObject(raw []byte) (map[string]any, error) {
	var value any
	if err := yaml.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return map[string]any{}, nil
	}
	encoded, err := json.Marshal(normalizeYAML(value))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(encoded, &out); err != nil || out == nil {
		return nil, errors.New("not an object")
	}
	return out, nil
}
