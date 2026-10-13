package registry

import (
	"orchestrator/internal/ports/moduleregistry"

	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"orchestrator/internal/domain/tfmodule"
)

// ErrContract marks a module the registry refuses after static inspection:
// forbidden constructs, providers outside the release, unsupported types or
// a module that cannot run with the pinned release.
var ErrContract = fmt.Errorf("%w: module contract is not allowed", moduleregistry.ErrRejected)

func contractError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrContract, fmt.Sprintf(format, args...))
}

// moduleDir identifies one module directory: a path in one pinned source
// revision (the root's source, or an allowlisted Git source at a commit).
type moduleDir struct {
	sourceID, revision, path string
}

func (d moduleDir) id() string { return d.sourceID + "\x00" + d.revision + "\x00" + d.path }

// sourceResolver opens the snapshots a call tree needs: the root snapshot,
// and allowlisted Git sources at exact commits for remote child modules.
type sourceResolver interface {
	tree(dir moduleDir) (Tree, error)
	// remote parses a remote module call source and returns the pinned
	// directory it names, or a refusal for unapproved or mutable sources.
	remote(source string) (moduleDir, error)
}

// inspection is the result of inspecting one root module and every module
// it calls, local or remote.
type inspection struct {
	contract  tfmodule.Contract
	providers map[string]tfmodule.ProviderRequirement
	trees     map[moduleDir]Tree
}

// fileFunctions read files; they are allowed only on a literal path below
// path.module so a module never reads outside its own verified content.
var fileFunctions = map[string]bool{
	"file": true, "fileexists": true, "fileset": true, "filebase64": true,
	"filebase64sha256": true, "filebase64sha512": true, "filemd5": true,
	"filesha1": true, "filesha256": true, "filesha512": true, "templatefile": true,
}

// forbiddenFunctions resolve host paths outside the module.
var forbiddenFunctions = map[string]bool{"abspath": true, "pathexpand": true}

// moduleFile is one parsed top-level .tf file.
type moduleFile struct {
	name string
	body *hclsyntax.Body
	src  []byte
}

type moduleCall struct {
	name, source string
	target       moduleDir
}

type moduleInfo struct {
	requiredVersion []string
	providers       map[string]string // local name -> fully qualified source
	constraints     map[string]string // local name -> version constraints
	inputs          map[string]tfmodule.Input
	outputs         map[string]tfmodule.Output
	calls           []moduleCall
}

// maxModuleCalls bounds the call tree of one bundle.
const maxModuleCalls = 256

// inspect statically checks a root module and its whole module call tree.
func inspect(sources sourceResolver, root moduleDir, release Release, kind string, resourceTypes, executorOutputs []string) (inspection, error) {
	result := inspection{trees: map[moduleDir]Tree{}, providers: map[string]tfmodule.ProviderRequirement{}}
	infos := map[moduleDir]moduleInfo{}
	type pending struct {
		dir       moduleDir
		key       string
		source    string
		ancestors map[moduleDir]bool
	}
	queue := []pending{{dir: root, ancestors: map[moduleDir]bool{}}}
	type call struct {
		key, source string
		dir         moduleDir
	}
	var calls []call
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if item.key != "" {
			calls = append(calls, call{key: item.key, source: item.source, dir: item.dir})
			if len(calls) > maxModuleCalls {
				return inspection{}, contractError("the module call tree is too large")
			}
		}
		info, done := infos[item.dir]
		if !done {
			tree, err := sources.tree(item.dir)
			if err != nil {
				return inspection{}, err
			}
			info, err = inspectModule(item.dir, tree, sources)
			if err != nil {
				return inspection{}, err
			}
			infos[item.dir] = info
			result.trees[item.dir] = tree
		}
		ancestors := map[moduleDir]bool{item.dir: true}
		for dir := range item.ancestors {
			ancestors[dir] = true
		}
		for _, c := range info.calls {
			if ancestors[c.target] {
				return inspection{}, contractError("module calls form a cycle")
			}
			key := c.name
			if item.key != "" {
				key = item.key + "." + c.name
			}
			queue = append(queue, pending{dir: c.target, key: key, source: c.source, ancestors: ancestors})
		}
	}

	rootInfo := infos[root]
	rootSources := map[string]string{}
	for local, source := range rootInfo.providers {
		rootSources[source] = local
	}
	dirs := make([]moduleDir, 0, len(infos))
	for dir := range infos {
		dirs = append(dirs, dir)
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].id() < dirs[j].id() })
	for _, dir := range dirs {
		info := infos[dir]
		for _, constraint := range info.requiredVersion {
			if ok, err := satisfies(release.Terraform.Version, constraint); err != nil || !ok {
				return inspection{}, contractError("a module required_version does not admit the release Terraform version")
			}
		}
		for local, source := range info.providers {
			pin, ok := release.provider(source)
			if !ok {
				return inspection{}, contractError("provider %q is not part of the platform release", local)
			}
			if ok, err := satisfies(pin.Version, info.constraints[local]); err != nil || !ok {
				return inspection{}, contractError("provider %q constraints do not admit the release version", local)
			}
			if _, inRoot := rootSources[source]; !inRoot {
				// A provider configured only below the root would run with
				// an implicit default configuration and ambient credentials.
				return inspection{}, contractError("provider %q of a called module must also be required by the root module", local)
			}
		}
	}
	for local, source := range rootInfo.providers {
		pin, _ := release.provider(source)
		result.providers[local] = tfmodule.ProviderRequirement{
			Source: source, Version: pin.Version, Constraints: rootInfo.constraints[local],
			Binding: pin.Binding, Hashes: append([]string(nil), pin.Hashes...),
		}
	}

	owned := []string{}
	for _, name := range release.ExecutorVariables {
		if _, ok := rootInfo.inputs[name]; ok {
			owned = append(owned, name)
		}
	}
	transitive := []tfmodule.TransitiveModule{}
	index := map[moduleDir]int{}
	for _, dir := range dirs {
		if dir == root {
			continue // pinned by the root content digest
		}
		index[dir] = len(transitive)
		transitive = append(transitive, tfmodule.TransitiveModule{SourceID: dir.sourceID, Revision: dir.revision, Path: dir.path, ContentSHA256: result.trees[dir].Digest()})
	}
	moduleCalls := []tfmodule.ModuleCall{}
	for _, c := range calls {
		i, ok := index[c.dir]
		if !ok {
			return inspection{}, contractError("a module calls the root module")
		}
		moduleCalls = append(moduleCalls, tfmodule.ModuleCall{Key: c.key, Source: c.source, Transitive: i})
	}
	sort.Slice(moduleCalls, func(i, j int) bool { return moduleCalls[i].Key < moduleCalls[j].Key })
	types := append([]string{}, resourceTypes...)
	sort.Strings(types)
	outputs := append([]string{}, executorOutputs...)
	sort.Strings(outputs)
	for _, name := range outputs {
		if _, clash := rootInfo.outputs[name]; clash {
			return inspection{}, contractError("executor output %q collides with a module output", name)
		}
	}
	result.contract = tfmodule.Contract{
		SchemaVersion:   tfmodule.ContractSchemaVersion,
		SourceKind:      kind,
		ResourceTypes:   types,
		RequiredVersion: strings.Join(rootInfo.requiredVersion, ", "),
		Inputs:          rootInfo.inputs,
		Outputs:         rootInfo.outputs,
		ExecutorOwned:   owned,
		ExecutorOutputs: outputs,
		Transitive:      transitive,
		ModuleCalls:     moduleCalls,
	}
	return result, nil
}

// inspectModule parses the top-level .tf files of one module directory.
func inspectModule(dir moduleDir, tree Tree, sources sourceResolver) (moduleInfo, error) {
	info := moduleInfo{providers: map[string]string{}, constraints: map[string]string{}, inputs: map[string]tfmodule.Input{}, outputs: map[string]tfmodule.Output{}}
	var files []moduleFile
	names := make([]string, 0, len(tree))
	for rel := range tree {
		if !strings.Contains(rel, "/") && strings.HasSuffix(rel, ".tf") {
			names = append(names, rel)
		}
	}
	if len(names) == 0 {
		return moduleInfo{}, contractError("a module directory has no .tf file")
	}
	sort.Strings(names)
	parser := hclparse.NewParser()
	for _, name := range names {
		file, diags := parser.ParseHCL(tree[name], name)
		if diags.HasErrors() {
			return moduleInfo{}, contractError("a module file is not valid HCL")
		}
		body, ok := file.Body.(*hclsyntax.Body)
		if !ok {
			return moduleInfo{}, contractError("a module file is not native HCL syntax")
		}
		files = append(files, moduleFile{name: name, body: body, src: tree[name]})
	}

	for _, f := range files {
		if err := checkExpressions(f.body); err != nil {
			return moduleInfo{}, err
		}
		if len(f.body.Attributes) > 0 {
			return moduleInfo{}, contractError("top-level attributes are not valid Terraform configuration")
		}
		for _, block := range f.body.Blocks {
			var err error
			switch block.Type {
			case "terraform":
				err = readTerraformBlock(block, &info)
			case "provider":
				err = checkProviderBlock(block)
			case "variable":
				err = readVariableBlock(block, f.src, &info)
			case "output":
				err = readOutputBlock(block, &info)
			case "resource", "data":
				err = checkResourceBlock(block)
			case "module":
				err = readModuleCall(dir, block, &info, sources)
			case "locals", "moved", "check":
			default:
				err = contractError("%s blocks are not allowed in a registered module", block.Type)
			}
			if err != nil {
				return moduleInfo{}, err
			}
		}
	}
	// Every resource type must belong to a provider the module declares, so
	// no undeclared (implicitly sourced) provider plugin can run.
	for _, f := range files {
		for _, block := range f.body.Blocks {
			if block.Type != "resource" && block.Type != "data" {
				continue
			}
			local := resourceProvider(block)
			if local == "terraform" {
				continue
			}
			if _, ok := info.providers[local]; !ok {
				return moduleInfo{}, contractError("a resource uses provider %q that the module does not declare in required_providers", local)
			}
		}
	}
	return info, nil
}

func readTerraformBlock(block *hclsyntax.Block, info *moduleInfo) error {
	for name, attr := range block.Body.Attributes {
		if name != "required_version" {
			return contractError("terraform.%s is not allowed in a registered module", name)
		}
		text, ok := literalString(attr.Expr)
		if !ok {
			return contractError("terraform.required_version must be a literal string")
		}
		info.requiredVersion = append(info.requiredVersion, text)
	}
	for _, inner := range block.Body.Blocks {
		if inner.Type != "required_providers" {
			// backend/cloud would move state out of the platform backend.
			return contractError("terraform.%s is not allowed in a registered module", inner.Type)
		}
		for local, attr := range inner.Body.Attributes {
			source, constraints := "hashicorp/"+local, ""
			switch expr := attr.Expr.(type) {
			case *hclsyntax.ObjectConsExpr:
				for _, item := range expr.Items {
					key := hcl.ExprAsKeyword(item.KeyExpr)
					switch key {
					case "source":
						text, ok := literalString(item.ValueExpr)
						if !ok {
							return contractError("required_providers.%s.source must be a literal", local)
						}
						source = text
					case "version":
						text, ok := literalString(item.ValueExpr)
						if !ok {
							return contractError("required_providers.%s.version must be a literal", local)
						}
						constraints = text
					case "configuration_aliases":
					default:
						return contractError("required_providers.%s has an unsupported field", local)
					}
				}
			default:
				return contractError("required_providers.%s must be an object with source and version", local)
			}
			qualified, err := qualifyProvider(source)
			if err != nil {
				return contractError("required_providers.%s source is not a public registry address", local)
			}
			info.providers[local] = qualified
			info.constraints[local] = constraints
		}
	}
	return nil
}

func qualifyProvider(source string) (string, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(source)), "/")
	switch {
	case len(parts) == 2 && parts[0] != "" && parts[1] != "":
		return "registry.terraform.io/" + parts[0] + "/" + parts[1], nil
	case len(parts) == 3 && parts[0] == "registry.terraform.io" && parts[1] != "" && parts[2] != "":
		return strings.Join(parts, "/"), nil
	}
	return "", errors.New("unsupported provider source")
}

// checkProviderBlock allows only the region (from a module variable) and
// default tags. Credentials, profiles, endpoints, assume-role, kubeconfig,
// hosts and exec plugins are executor-owned bindings, never module code.
func checkProviderBlock(block *hclsyntax.Block) error {
	for name, attr := range block.Body.Attributes {
		switch name {
		case "alias":
		case "region":
			traversal, ok := attr.Expr.(*hclsyntax.ScopeTraversalExpr)
			if !ok || traversal.Traversal.RootName() != "var" {
				return contractError("a provider region must come from a module variable")
			}
		default:
			return contractError("provider.%s overrides the platform provider binding", name)
		}
	}
	for _, inner := range block.Body.Blocks {
		if inner.Type != "default_tags" {
			return contractError("provider.%s overrides the platform provider binding", inner.Type)
		}
	}
	return nil
}

func readVariableBlock(block *hclsyntax.Block, src []byte, info *moduleInfo) error {
	if len(block.Labels) != 1 {
		return contractError("a variable block needs one name")
	}
	name := block.Labels[0]
	input := tfmodule.Input{Type: "any"}
	for attrName, attr := range block.Body.Attributes {
		switch attrName {
		case "type":
			canonical, err := tfmodule.CanonicalType(typeExpression(attr.Expr, src))
			if err != nil {
				return contractError("variable %q uses a type outside the supported subset", name)
			}
			input.Type = canonical
		case "default":
			value, diags := attr.Expr.Value(nil)
			if diags.HasErrors() {
				return contractError("variable %q default must be a literal", name)
			}
			input.HasDefault = true
			input.Default = ctyToGo(value)
		case "sensitive":
			flag, ok := literalBool(attr.Expr)
			if !ok {
				return contractError("variable %q sensitive must be a literal bool", name)
			}
			input.Sensitive = flag
		case "description", "nullable":
		default:
			return contractError("variable %q uses an unsupported attribute", name)
		}
	}
	for _, inner := range block.Body.Blocks {
		if inner.Type != "validation" {
			return contractError("variable %q uses an unsupported block", name)
		}
	}
	if input.Sensitive && input.HasDefault && input.Default != nil {
		// A sensitive default is a credential in module code; it would land
		// in registry metadata. Sensitive values come from the executor.
		return contractError("sensitive variable %q must not declare a default value", name)
	}
	input.Required = !input.HasDefault
	info.inputs[name] = input
	return nil
}

func readOutputBlock(block *hclsyntax.Block, info *moduleInfo) error {
	if len(block.Labels) != 1 {
		return contractError("an output block needs one name")
	}
	name := block.Labels[0]
	output := tfmodule.Output{}
	value, ok := block.Body.Attributes["value"]
	if !ok {
		return contractError("output %q has no value", name)
	}
	output.Type = inferType(value.Expr)
	for attrName, attr := range block.Body.Attributes {
		switch attrName {
		case "value", "description", "depends_on":
		case "sensitive":
			flag, ok := literalBool(attr.Expr)
			if !ok {
				return contractError("output %q sensitive must be a literal bool", name)
			}
			output.Sensitive = flag
		default:
			return contractError("output %q uses an unsupported attribute", name)
		}
	}
	for _, inner := range block.Body.Blocks {
		if inner.Type != "precondition" {
			return contractError("output %q uses an unsupported block", name)
		}
	}
	info.outputs[name] = output
	return nil
}

// checkResourceBlock refuses provisioners, connections and data sources that
// read other state.
func checkResourceBlock(block *hclsyntax.Block) error {
	if len(block.Labels) != 2 {
		return contractError("a %s block needs a type and a name", block.Type)
	}
	if block.Type == "data" && block.Labels[0] == "terraform_remote_state" {
		return contractError("terraform_remote_state data sources are not allowed")
	}
	var walk func(body *hclsyntax.Body) error
	walk = func(body *hclsyntax.Body) error {
		for _, inner := range body.Blocks {
			switch inner.Type {
			case "provisioner":
				return contractError("provisioners are not allowed in a registered module")
			case "connection":
				return contractError("connection blocks are not allowed in a registered module")
			}
			if err := walk(inner.Body); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(block.Body)
}

// resourceProvider returns the provider local name of a resource or data
// block: the explicit provider reference or the type prefix.
func resourceProvider(block *hclsyntax.Block) string {
	if attr, ok := block.Body.Attributes["provider"]; ok {
		if traversal, ok := attr.Expr.(*hclsyntax.ScopeTraversalExpr); ok {
			return traversal.Traversal.RootName()
		}
	}
	typ := block.Labels[0]
	if idx := strings.Index(typ, "_"); idx > 0 {
		return typ[:idx]
	}
	return typ
}

// readModuleCall accepts a literal relative source inside the same pinned
// revision, or a remote child at an Admin-allowlisted HTTPS Git source and an
// exact full commit. Registry, unallowlisted, mutable and other remote forms
// are refused. Every called directory is inspected and pinned in the bundle.
func readModuleCall(dir moduleDir, block *hclsyntax.Block, info *moduleInfo, sources sourceResolver) error {
	if len(block.Labels) != 1 {
		return contractError("a module block needs one name")
	}
	attr, ok := block.Body.Attributes["source"]
	if !ok {
		return contractError("a module call has no source")
	}
	source, ok := literalString(attr.Expr)
	if !ok {
		return contractError("a module call source must be a literal")
	}
	if _, versioned := block.Body.Attributes["version"]; versioned {
		return contractError("registry module calls are not allowed")
	}
	for _, name := range []string{"count", "for_each"} {
		if _, ok := block.Body.Attributes[name]; ok {
			// Instance keys would not match the pinned module call keys.
			return contractError("module calls with %s are not supported", name)
		}
	}
	call := moduleCall{name: block.Labels[0], source: source}
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
		target := path.Clean(path.Join(dir.path, source))
		if target == ".." || strings.HasPrefix(target, "../") || strings.HasPrefix(target, "/") {
			return contractError("a module call escapes the pinned source")
		}
		if target != "." {
			if err := checkPath(target); err != nil {
				return contractError("a module call path is not a clean source path")
			}
		}
		call.target = moduleDir{sourceID: dir.sourceID, revision: dir.revision, path: target}
	} else {
		target, err := sources.remote(source)
		if err != nil {
			return err
		}
		call.target = target
	}
	info.calls = append(info.calls, call)
	return nil
}

// parseRemoteSource reads `git::https://host/repo//sub/dir?ref=<full commit>`.
// The repository URL must equal an allowlisted Git source URL exactly; the
// only query parameter is ref, a full commit ID.
func parseRemoteSource(source string, policy Policy) (moduleDir, error) {
	refuse := func() (moduleDir, error) {
		return moduleDir{}, contractError("a remote module call must use an allowlisted HTTPS Git source at a full commit (git::https://...//path?ref=<commit>)")
	}
	rest, ok := strings.CutPrefix(source, "git::")
	if !ok {
		return refuse()
	}
	u, err := url.Parse(rest)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Opaque != "" || u.Host == "" {
		return refuse()
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(query) != 1 || len(query["ref"]) != 1 || !tfmodule.FullCommit(query["ref"][0]) {
		return refuse()
	}
	repoPath, sub := u.Path, "."
	if idx := strings.Index(u.Path, "//"); idx >= 0 {
		repoPath, sub = u.Path[:idx], u.Path[idx+2:]
		if sub == "" {
			sub = "."
		}
	}
	if !tfmodule.ValidSubdirectory(sub) {
		return refuse()
	}
	repo := "https://" + u.Host + repoPath
	for _, s := range policy.Sources {
		if s.URL == repo {
			return moduleDir{sourceID: s.ID, revision: query["ref"][0], path: sub}, nil
		}
	}
	return refuse()
}

// checkExpressions refuses file reads outside path.module, host path
// functions and path.cwd/path.root references.
func checkExpressions(body *hclsyntax.Body) error {
	var failure error
	hclsyntax.VisitAll(body, func(node hclsyntax.Node) hcl.Diagnostics {
		if failure != nil {
			return nil
		}
		switch expr := node.(type) {
		case *hclsyntax.FunctionCallExpr:
			switch {
			case forbiddenFunctions[expr.Name]:
				failure = contractError("function %s reads host paths", expr.Name)
			case fileFunctions[expr.Name]:
				if len(expr.Args) == 0 || !moduleRelativePath(expr.Args[0]) {
					failure = contractError("function %s may only read a literal path below path.module", expr.Name)
				}
			}
		case *hclsyntax.ScopeTraversalExpr:
			if expr.Traversal.RootName() == "path" && len(expr.Traversal) > 1 {
				if attr, ok := expr.Traversal[1].(hcl.TraverseAttr); ok && (attr.Name == "cwd" || attr.Name == "root") {
					failure = contractError("path.%s refers outside the module", attr.Name)
				}
			}
		}
		return nil
	})
	return failure
}

// moduleRelativePath accepts "${path.module}/<clean relative path>".
func moduleRelativePath(expr hclsyntax.Expression) bool {
	template, ok := expr.(*hclsyntax.TemplateExpr)
	if !ok || len(template.Parts) != 2 {
		return false
	}
	traversal, ok := template.Parts[0].(*hclsyntax.ScopeTraversalExpr)
	if !ok || traversal.Traversal.RootName() != "path" || len(traversal.Traversal) != 2 {
		return false
	}
	if attr, ok := traversal.Traversal[1].(hcl.TraverseAttr); !ok || attr.Name != "module" {
		return false
	}
	rest, ok := literalString(template.Parts[1])
	if !ok || !strings.HasPrefix(rest, "/") {
		return false
	}
	return checkPath(strings.TrimPrefix(rest, "/")) == nil
}

func literalString(expr hclsyntax.Expression) (string, bool) {
	switch e := expr.(type) {
	case *hclsyntax.TemplateExpr:
		if !e.IsStringLiteral() {
			return "", false
		}
		value, diags := e.Value(nil)
		if diags.HasErrors() || value.Type() != cty.String || value.IsNull() {
			return "", false
		}
		return value.AsString(), true
	case *hclsyntax.LiteralValueExpr:
		if e.Val.Type() != cty.String || e.Val.IsNull() {
			return "", false
		}
		return e.Val.AsString(), true
	}
	return "", false
}

func literalBool(expr hclsyntax.Expression) (bool, bool) {
	literal, ok := expr.(*hclsyntax.LiteralValueExpr)
	if !ok || literal.Val.Type() != cty.Bool || literal.Val.IsNull() {
		return false, false
	}
	return literal.Val.True(), true
}

func typeExpression(expr hclsyntax.Expression, src []byte) string {
	if traversal, ok := expr.(*hclsyntax.ScopeTraversalExpr); ok && len(traversal.Traversal) == 1 {
		return traversal.Traversal.RootName()
	}
	rng := expr.Range()
	if rng.Start.Byte >= 0 && rng.End.Byte <= len(src) && rng.Start.Byte <= rng.End.Byte {
		return strings.Join(strings.Fields(string(src[rng.Start.Byte:rng.End.Byte])), "")
	}
	return ""
}

// inferType derives an output type when the expression states it; otherwise
// the type is unknown ("") and checked against the contract after apply.
func inferType(expr hclsyntax.Expression) string {
	switch e := expr.(type) {
	case *hclsyntax.LiteralValueExpr:
		switch e.Val.Type() {
		case cty.String:
			return "string"
		case cty.Number:
			return "number"
		case cty.Bool:
			return "bool"
		}
	case *hclsyntax.TemplateExpr:
		return "string"
	case *hclsyntax.SplatExpr, *hclsyntax.TupleConsExpr:
		return "list(any)"
	case *hclsyntax.FunctionCallExpr:
		switch e.Name {
		case "tostring", "format", "join", "lower", "upper", "trimspace", "jsonencode", "base64encode":
			return "string"
		case "tonumber", "length":
			return "number"
		case "tobool":
			return "bool"
		case "tolist", "toset", "concat", "compact", "distinct", "sort":
			return "list(any)"
		case "tomap":
			return "map(any)"
		}
	}
	return ""
}

// ctyToGo converts a literal default into its JSON-compatible Go value.
func ctyToGo(value cty.Value) any {
	if value.IsNull() || !value.IsKnown() {
		return nil
	}
	t := value.Type()
	switch {
	case t == cty.String:
		return value.AsString()
	case t == cty.Number:
		f, _ := value.AsBigFloat().Float64()
		return f
	case t == cty.Bool:
		return value.True()
	case t.IsListType() || t.IsSetType() || t.IsTupleType():
		out := []any{}
		for it := value.ElementIterator(); it.Next(); {
			_, v := it.Element()
			out = append(out, ctyToGo(v))
		}
		return out
	case t.IsMapType() || t.IsObjectType():
		out := map[string]any{}
		for it := value.ElementIterator(); it.Next(); {
			k, v := it.Element()
			out[k.AsString()] = ctyToGo(v)
		}
		return out
	}
	return nil
}
