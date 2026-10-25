package renderengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"text/template/parse"

	"gopkg.in/yaml.v3"

	"orchestrator/internal/domain/renderbundle"
)

// Provisioner fields score-k8s 0.15.0 template provisioners may use in a Git
// bundle: output bridging only. init/state/shared/manifests/directories and
// cmd:// provisioners are refused (RB-C04).
var provisionerFields = map[string]bool{"uri": true, "type": true, "class": true, "id": true, "description": true, "outputs": true}

// Bundle is one validated bundle tree: the contract plus the file contents
// the stages read. It never holds a host path.
type Bundle struct {
	Contract renderbundle.Contract
	Files    map[string][]byte
}

// File returns the content of a bundle-relative file.
func (b Bundle) File(name string) []byte { return b.Files[name] }

// Validate checks a complete selected tree against version 1: limits,
// manifest, schema, defaults, template syntax and helper names, and
// provisioner restrictions. It executes no author template.
func Validate(files map[string][]byte) (Bundle, error) {
	if len(files) > renderbundle.MaxFiles {
		return Bundle{}, invalid("", "the bundle has more than %d files", renderbundle.MaxFiles)
	}
	total := 0
	for name, content := range files {
		if !renderbundle.ValidFilePath(name) {
			return Bundle{}, invalid("", "a file path is not a clean relative path")
		}
		if len(content) > renderbundle.MaxFileBytes {
			return Bundle{}, invalid(name, "the file is larger than 1 MiB")
		}
		total += len(content)
	}
	if total > renderbundle.MaxTreeBytes {
		return Bundle{}, invalid("", "the bundle is larger than 4 MiB")
	}
	raw, ok := files[renderbundle.ManifestFile]
	if !ok {
		return Bundle{}, invalid(renderbundle.ManifestFile, "the manifest is missing")
	}
	manifest, err := renderbundle.ParseManifest(raw)
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
	if err := renderbundle.ValidateSchema(schema); err != nil {
		var invalidErr *renderbundle.InvalidError
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
	templates := append(append([]string{manifest.Normalize}, manifest.Resources...), manifest.Patches...)
	for _, name := range templates {
		source, err := need(name)
		if err != nil {
			return Bundle{}, err
		}
		if _, err := Parse(name, string(source)); err != nil {
			return Bundle{}, invalid(name, "the template does not parse with the git-render-v1 helpers (line %d)", line(err))
		}
	}
	for _, name := range manifest.Provisioners {
		content, err := need(name)
		if err != nil {
			return Bundle{}, err
		}
		if !strings.HasSuffix(name, ".provisioners.yaml") {
			return Bundle{}, invalid(name, "provisioner files must end with .provisioners.yaml")
		}
		if err := validateProvisioners(name, content); err != nil {
			return Bundle{}, err
		}
	}
	return Bundle{Contract: renderbundle.Contract{Manifest: manifest, InputsSchema: schema, Defaults: defaults, InputContract: manifest.InputContract}, Files: files}, nil
}

func invalid(file, format string, args ...any) error {
	return &renderbundle.InvalidError{File: file, Reason: fmt.Sprintf(format, args...)}
}

func validateProvisioners(name string, content []byte) error {
	var entries []map[string]any
	if err := yaml.Unmarshal(content, &entries); err != nil {
		return invalid(name, "the provisioner file is not a YAML list without duplicate keys")
	}
	if len(entries) == 0 {
		return invalid(name, "the provisioner file declares no provisioner")
	}
	for _, entry := range entries {
		fields := make([]string, 0, len(entry))
		for key := range entry {
			fields = append(fields, key)
		}
		sort.Strings(fields)
		for _, key := range fields {
			if !provisionerFields[key] {
				return invalid(name, "provisioners may declare only uri, type, class, id, description and outputs")
			}
		}
		uri, _ := entry["uri"].(string)
		parsed, err := url.Parse(uri)
		if err != nil || parsed.Scheme != "template" || uri == "template://" {
			return invalid(name, "only template:// output provisioners are supported")
		}
		if typ, _ := entry["type"].(string); typ == "" {
			return invalid(name, "every provisioner needs a type")
		}
		for _, key := range []string{"class", "id", "description"} {
			if value, ok := entry[key]; ok {
				if _, text := value.(string); !text {
					return invalid(name, "provisioner %s must be a string", key)
				}
			}
		}
		outputs, ok := entry["outputs"].(string)
		if !ok {
			return invalid(name, "every provisioner needs an outputs template")
		}
		// score-k8s owns the provisioner function set (pinned by version), so
		// only the syntax is checked here.
		tree := parse.New(name)
		tree.Mode = parse.SkipFuncCheck
		if _, err := tree.Parse(outputs, "", "", map[string]*parse.Tree{}); err != nil {
			return invalid(name, "a provisioner outputs template does not parse")
		}
	}
	return nil
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
