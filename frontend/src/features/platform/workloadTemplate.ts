import { provisionToRows, rowsToProvision, type ProvisionRow } from './typedProvision';
import { parseObjectText, type JsonObject } from './typedVariables';
import { t } from '../../shared/i18n';

// Typed authoring of a workload (score-k8s) Definition (ADR-016): a literal
// render_bundle, whole namespace/cluster output references, the inputs the
// and provision (companion) rules. The workload driver accepts no other variable
// (nothing at execution would consume it), so the form offers none. The typed form
// and the advanced JSON serialize to the same two objects.

// inputs is the literal render_inputs object of a Git bundle (ADR-021); undefined
// means the Definition carries no render_inputs key (installed bundles accept none).
export type TemplateForm = { bundle: string; namespaceRef: string; clusterRef: string; rows: ProvisionRow[]; inputs?: JsonObject };
export const emptyTemplateForm = (): TemplateForm => ({ bundle: '', namespaceRef: '', clusterRef: '', rows: [] });

// Form -> the two objects sent to the server. A problem (Vietnamese) blocks Save. Both references empty
// is the render-only form the server accepts only for AWS legacy Applications; one alone is never valid.
export function templateToBody(form: TemplateForm): { variables?: JsonObject; provision?: JsonObject; problem?: string } {
  if (form.bundle === '') return { problem: t('platform.workloadTemplate.chooseARenderingBundle') };
  const namespace = form.namespaceRef.trim() !== '';
  const cluster = form.clusterRef.trim() !== '';
  if (namespace !== cluster) return { problem: t('platform.workloadTemplate.enterBothTheNamespaceAnd') };
  const variables: JsonObject = { render_bundle: form.bundle };
  if (namespace) { variables.namespace = form.namespaceRef; variables.cluster = form.clusterRef; }
  if (form.inputs !== undefined) variables.render_inputs = form.inputs;
  const provision = rowsToProvision(form.rows);
  if (!provision.value) return { problem: provision.problem };
  return { variables, ...(Object.keys(provision.value).length ? { provision: provision.value } : {}) };
}

// Server/JSON -> typed form; a reason keeps the advanced JSON authoritative.
export function bodyToTemplate(variables: unknown, provision: unknown): { form?: TemplateForm; reason?: string } {
  if (variables === null || typeof variables !== 'object' || Array.isArray(variables)) return { reason: t('platform.workloadTemplate.variablesIsNotAJSON') };
  const source = variables as JsonObject;
  const form = emptyTemplateForm();
  for (const [name, value] of Object.entries(source)) {
    if (name === 'render_inputs') {
      if (value === null || typeof value !== 'object' || Array.isArray(value)) return { reason: t('platform.workloadTemplate.renderInputsMustBeAnObject') };
      form.inputs = value as JsonObject;
      continue;
    }
    if (name === 'render_bundle' || name === 'namespace' || name === 'cluster') {
      if (typeof value !== 'string') return { reason: t('platform.workloadTemplate.nameMustBeAString', { name }) };
      if (name === 'render_bundle') form.bundle = value; else if (name === 'namespace') form.namespaceRef = value; else form.clusterRef = value;
      continue;
    }
    return { reason: t('platform.workloadTemplate.variableNameIsNotSupported', { name }) };
  }
  const text = provision === undefined ? '{}' : JSON.stringify(provision);
  const rows = provisionToRows(text);
  if (!rows.rows) return { reason: rows.reason };
  form.rows = rows.rows;
  return { form };
}

export function jsonToBody(text: string): { variables?: unknown; provision?: unknown; error?: string } {
  const parsed = parseObjectText(text, t('platform.workloadTemplate.advancedJSON'));
  if (!parsed.value) return { error: parsed.error };
  for (const key of Object.keys(parsed.value)) if (key !== 'variables' && key !== 'provision') return { error: t('platform.workloadTemplate.fieldKeyIsNotSupported', { key }) };
  return { variables: parsed.value.variables, provision: parsed.value.provision };
}
