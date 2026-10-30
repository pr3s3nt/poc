import { assertSameDefinition } from './render-bundle-config.mjs';

// Opt-in migration changes only immutable source pins. It must not silently
// change criteria, account, placement, variables, mappings or resource identity.
export function definitionAction(existing, desired, updateExisting = false) {
  if (!existing) return 'create';
  try { assertSameDefinition(existing, desired); return 'reuse'; } catch (error) {
    if (!updateExisting) throw error;
  }
  if (!Number.isInteger(existing.version) || existing.version < 1) throw Error(`${desired.key}: thiếu version để cập nhật CAS`);
  const stripSource = definition => {
    const copy = structuredClone(definition);
    const values = copy.driverInputs?.values;
    if (copy.driverType === 'terraform' && values) delete values.source;
    else if (copy.driverType === 'score-k8s' && values?.variables) delete values.variables.render_bundle;
    else throw Error(`${desired.key}: --update-existing chỉ đổi source Terraform/render bundle`);
    return copy;
  };
  if (desired.driverType === 'terraform' && !desired.driverInputs?.values?.source) throw Error(`${desired.key}: update cần verified Git module pin`);
  try { assertSameDefinition(stripSource(existing), stripSource(desired)); } catch {
    throw Error(`${desired.key}: có thay đổi ngoài Git source/pin; giữ cấu hình hiện tại và kiểm tra thủ công`);
  }
  return 'update';
}
