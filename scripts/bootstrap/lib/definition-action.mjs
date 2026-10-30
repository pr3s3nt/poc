import { isDeepStrictEqual } from 'node:util';
import { differences, preserveConsoleVariables } from './config-diff.mjs';
import { assertSameDefinition } from './render-bundle-config.mjs';

// Source migration preserves Console-only variables and adds seed-only ones.
// Conflicting shared variables and other non-source changes remain refused.
export function definitionAction(existing, desired, updateExisting = false) {
  if (!existing) return 'create';
  desired = preserveConsoleVariables(existing, desired, updateExisting);
  try { assertSameDefinition(existing, desired); return 'reuse'; } catch (error) {
    if (!updateExisting) throw error;
  }
  if (!Number.isInteger(existing.version) || existing.version < 1) throw Error(`${desired.key}: thiếu version để cập nhật CAS`);
  const currentVariables = existing.driverInputs?.values?.variables ?? {};
  const desiredVariables = desired.driverInputs?.values?.variables ?? {};
  const conflicts = Object.keys(currentVariables).filter(name => Object.hasOwn(desiredVariables, name) && !(desired.driverType === 'score-k8s' && name === 'render_bundle') && !isDeepStrictEqual(currentVariables[name], desiredVariables[name]));
  if (conflicts.length) throw Error(`${desired.key}: có thay đổi ngoài Git source/pin; biến cùng khai báo có giá trị khác nhau. ${conflicts.map(name => differences(currentVariables[name], desiredVariables[name], `driverInputs.values.variables[${JSON.stringify(name)}]`)).join('; ')}`);
  const stripSource = definition => {
    const copy = structuredClone(definition);
    const values = copy.driverInputs?.values;
    if (copy.driverType === 'terraform' && values) delete values.source;
    else if (copy.driverType === 'score-k8s' && values?.variables) delete values.variables.render_bundle;
    else throw Error(`${desired.key}: --update-existing chỉ đổi source Terraform/render bundle`);
    // Shared entries were compared above; additions are permitted and Console
    // extras are present in the merged desired used by the payload guard.
    delete values.variables;
    return copy;
  };
  if (desired.driverType === 'terraform' && !desired.driverInputs?.values?.source) throw Error(`${desired.key}: update cần verified Git module pin`);
  try { assertSameDefinition(stripSource(existing), stripSource(desired)); } catch (error) {
    throw Error(`${desired.key}: có thay đổi ngoài Git source/pin; giữ cấu hình hiện tại. ${error.message}`);
  }
  return 'update';
}
