import type { Capability, CapabilityOutput, CapabilityProvider } from './capabilities';
import { baseKind, type JsonObject } from './typedVariables';

// Typed authoring of the Terraform mappings of a Definition (UC-03 BR-11, module
// contract "Driver Inputs shape"): input_mapping, output_mapping and
// provider_bindings. The typed form and the advanced JSON serialize to the
// same object, so a valid mapping survives every change of mode or module.
// Every choice comes from the verified module metadata and the Resource Type
// contract; nothing is guessed, defaulted or matched by name unless the author
// asks for it explicitly.

export type TypeContract = {
  key: string;
  inputs: { name: string; type: string; required?: boolean }[];
  outputs: { name: string; type: string; secret?: boolean; required?: boolean }[];
  classes: string[];
};

export type BindingDraft = { target: '' | 'environment' | 'resource'; clusterClass: string };
export type MappingDraft = {
  inputs: Record<string, string>; // Resource Type input -> module variable
  outputs: Record<string, string>; // Resource Type output -> module output
  bindings: Record<string, BindingDraft>; // provider requirement -> binding
};

export const emptyMapping = (): MappingDraft => ({ inputs: {}, outputs: {}, bindings: {} });
export const clusterType = 'k8s-cluster';
// The EKS cluster node of the Environment, addressed by a class the catalog serves.
export const clusterRef = (clusterClass: string) => `${clusterType}.${clusterClass}#environments.@app.@env`;
const clusterRefPattern = /^k8s-cluster\.([^#]+)#environments\.@app\.@env$/;

const has = (table: object, key: string) => Object.prototype.hasOwnProperty.call(table, key);

// Mirrors tfmodule.InputCompatible / OutputCompatible: an unknown ("any") side defers to binding time.
export function inputCompatible(contractType: string, moduleType: string): boolean {
  const want = contractType === '' ? 'any' : contractType;
  const kind = baseKind(moduleType);
  return want === 'any' || kind === 'any' || kind === want;
}
export function outputCompatible(contractType: string, moduleType: string | undefined): boolean {
  const want = contractType === '' ? 'any' : contractType;
  if (!moduleType || want === 'any') return true;
  const kind = baseKind(moduleType);
  return kind === 'any' || kind === want;
}

export const bindingProviders = (capability: Capability): CapabilityProvider[] => capability.providerRequirements.filter((provider) => provider.binding === 'aws' || provider.binding === 'kubernetes');
// The server publishes whether a pair needs a Driver Account (UC-03 V02-03); the
// form never infers it from the Resource Type or module name.
export const needsAccount = (capability: Capability): boolean => capability.account.required;
// A Kubernetes provider inherits its cluster from a binding target, never from an account.
export const inheritsCluster = (capability: Capability): boolean => capability.bindings.some((binding) => binding.kind === 'kubernetes');

export function mappingToObject(draft: MappingDraft, capability: Capability): { input_mapping: JsonObject; output_mapping: JsonObject; provider_bindings: JsonObject } {
  const input_mapping: JsonObject = {};
  for (const [param, variable] of Object.entries(draft.inputs)) if (variable !== '') input_mapping[variable] = { param };
  const output_mapping: JsonObject = {};
  for (const [typeOutput, moduleOutput] of Object.entries(draft.outputs)) if (moduleOutput !== '') output_mapping[moduleOutput] = typeOutput;
  const provider_bindings: JsonObject = {};
  for (const provider of bindingProviders(capability)) {
    const binding = draft.bindings[provider.name];
    if (provider.binding === 'aws') provider_bindings[provider.name] = { target: 'account' };
    else if (binding?.target === 'environment') provider_bindings[provider.name] = { target: 'environment' };
    else if (binding?.target === 'resource' && binding.clusterClass !== '') provider_bindings[provider.name] = { target: 'resource', resource_ref: clusterRef(binding.clusterClass) };
  }
  return { input_mapping, output_mapping, provider_bindings };
}

const isObject = (value: unknown): value is JsonObject => typeof value === 'object' && value !== null && !Array.isArray(value);

// Advanced JSON -> typed. A reason (Vietnamese) means the typed controls cannot
// hold the object safely and the JSON stays authoritative.
export function objectToMapping(value: JsonObject, capability: Capability, type: TypeContract | undefined, clusterClasses: string[]): { draft?: MappingDraft; reason?: string } {
  for (const key of Object.keys(value)) if (!['input_mapping', 'output_mapping', 'provider_bindings'].includes(key)) return { reason: `Trường "${key}" không có control typed. Giữ trong JSON nâng cao.` };
  const draft = emptyMapping();
  const variables = new Set(capability.variables.map((variable) => variable.name));
  const outputs = new Set(capability.outputs.map((output) => output.name));
  const inputMapping = value.input_mapping ?? {};
  if (!isObject(inputMapping)) return { reason: 'input_mapping không phải JSON object. Giữ trong JSON nâng cao.' };
  for (const [variable, entry] of Object.entries(inputMapping)) {
    const param = isObject(entry) && Object.keys(entry).length === 1 ? entry.param : undefined;
    if (typeof param !== 'string' || param === '') return { reason: `input_mapping."${variable}" không có dạng {param: …}. Giữ trong JSON nâng cao.` };
    if (!variables.has(variable)) return { reason: `Biến "${variable}" không có trong module đã chọn (hoặc do hệ thống sở hữu). Giữ trong JSON nâng cao.` };
    if (!type?.inputs.some((input) => input.name === param)) return { reason: `Tham số "${param}" không thuộc Loại tài nguyên. Giữ trong JSON nâng cao.` };
    if (has(draft.inputs, param)) return { reason: `Tham số "${param}" được ánh xạ hai lần. Giữ trong JSON nâng cao.` };
    draft.inputs[param] = variable;
  }
  const outputMapping = value.output_mapping ?? {};
  if (!isObject(outputMapping)) return { reason: 'output_mapping không phải JSON object. Giữ trong JSON nâng cao.' };
  for (const [moduleOutput, target] of Object.entries(outputMapping)) {
    if (typeof target !== 'string' || target === '') return { reason: `output_mapping."${moduleOutput}" phải là tên output của Loại tài nguyên. Giữ trong JSON nâng cao.` };
    if (!outputs.has(moduleOutput)) return { reason: `Output "${moduleOutput}" không có trong module đã chọn. Giữ trong JSON nâng cao.` };
    if (!type?.outputs.some((output) => output.name === target)) return { reason: `Output "${target}" không thuộc Loại tài nguyên. Giữ trong JSON nâng cao.` };
    if (has(draft.outputs, target)) return { reason: `Output "${target}" được ánh xạ hai lần. Giữ trong JSON nâng cao.` };
    draft.outputs[target] = moduleOutput;
  }
  const bindings = value.provider_bindings ?? {};
  if (!isObject(bindings)) return { reason: 'provider_bindings không phải JSON object. Giữ trong JSON nâng cao.' };
  const providers = new Map(bindingProviders(capability).map((provider) => [provider.name, provider]));
  for (const [name, entry] of Object.entries(bindings)) {
    const provider = providers.get(name);
    if (!provider) return { reason: `Provider "${name}" không nhận binding trong module đã chọn. Giữ trong JSON nâng cao.` };
    if (!isObject(entry)) return { reason: `provider_bindings."${name}" không phải JSON object. Giữ trong JSON nâng cao.` };
    if (provider.binding === 'aws') {
      if (entry.target === 'account' && Object.keys(entry).length === 1) continue;
      return { reason: `Binding của "${name}" phải là {target: account} (tài khoản của cấu hình). Giữ trong JSON nâng cao.` };
    }
    if (provider.binding === 'kubernetes' && entry.target === 'environment' && Object.keys(entry).length === 1) { draft.bindings[name] = { target: 'environment', clusterClass: '' }; continue; }
    const ref = typeof entry.resource_ref === 'string' ? clusterRefPattern.exec(entry.resource_ref) : null;
    if (provider.binding === 'kubernetes' && entry.target === 'resource' && Object.keys(entry).length === 2 && ref?.[1] && clusterClasses.includes(ref[1])) { draft.bindings[name] = { target: 'resource', clusterClass: ref[1] }; continue; }
    return { reason: `Binding của "${name}" không biểu diễn an toàn bằng control (cluster cần là class đã công bố). Giữ trong JSON nâng cao.` };
  }
  return { draft };
}

// ADR-017/catalog validateOutputMapping: the typed cluster target is produced
// by the platform from the completed provider identity, never a module mapping.
export const platformOutput = (capability: Capability, name: string): boolean => capability.resourceType === clusterType && name === 'cluster';

export type MappingProblem = { field: string; message: string };

// Guidance before POST. The server stays authoritative for every rule.
export function mappingProblems(draft: MappingDraft, capability: Capability, type: TypeContract | undefined): MappingProblem[] {
  const problems: MappingProblem[] = [];
  if (!type) return [{ field: 'type', message: 'Chưa có hợp đồng của Loại tài nguyên để ánh xạ.' }];
  for (const input of type.inputs) {
    if (!draft.inputs[input.name]) problems.push({ field: `input:${input.name}`, message: `Chọn biến module nhận tham số "${input.name}" của Loại tài nguyên.` });
  }
  for (const output of type.outputs) {
    if (output.required && !platformOutput(capability, output.name) && !draft.outputs[output.name]) problems.push({ field: `output:${output.name}`, message: `Chọn output module cung cấp output bắt buộc "${output.name}".` });
  }
  const clusterNeeded = bindingProviders(capability).filter((provider) => provider.binding === 'kubernetes');
  for (const provider of clusterNeeded) {
    const binding = draft.bindings[provider.name];
    if (!binding || binding.target === '') problems.push({ field: `binding:${provider.name}`, message: `Chọn nơi provider ${provider.name} lấy cluster: cluster do node EKS cung cấp, hoặc cluster của Môi trường (cách cũ, giữ tới V03).` });
    else if (binding.target === 'resource' && binding.clusterClass === '') problems.push({ field: `binding:${provider.name}`, message: `Chọn class cluster EKS cho provider ${provider.name}.` });
  }
  return problems;
}

// Variables fed by a Resource Type input: a param overrides the Definition
// fallback and the module default (module contract "Inputs và mapping").
export const mappedVariables = (draft: MappingDraft): Record<string, string> => {
  const out: Record<string, string> = {};
  for (const [param, variable] of Object.entries(draft.inputs)) if (variable !== '') out[variable] = param;
  return out;
};

export const outputOptions = (capability: Capability, contractType: string): CapabilityOutput[] => capability.outputs.filter((output) => outputCompatible(contractType, output.type));

export function sameNameInputs(capability: Capability, type: TypeContract): Record<string, string> {
  const out: Record<string, string> = {};
  for (const input of type.inputs) {
    const variable = capability.variables.find((candidate) => candidate.name === input.name);
    if (variable && inputCompatible(input.type, variable.type)) out[input.name] = variable.name;
  }
  return out;
}
export function sameNameOutputs(capability: Capability, type: TypeContract): Record<string, string> {
  const out: Record<string, string> = {};
  for (const output of type.outputs) {
    if (platformOutput(capability, output.name)) continue;
    const moduleOutput = capability.outputs.find((candidate) => candidate.name === output.name);
    if (moduleOutput && outputCompatible(output.type, moduleOutput.type)) out[output.name] = moduleOutput.name;
  }
  return out;
}
