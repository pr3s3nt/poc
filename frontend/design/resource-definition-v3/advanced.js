// Interactive prototype only: local draft, no API and no persistence.
const initialMappings = () => ({
  input_mapping: { database_name: { param: 'database' }, admin_username: { param: 'username' } },
  output_mapping: { endpoint: 'host', port: 'port', database_name: 'database', admin_username: 'username', password: 'password' },
  provider_bindings: { aws: { target: 'account' } }
});
const initialDependencies = () => [{ descriptor: 'vpc.default#@infra', params: '{}', is_dependent: false, match_dependents: false }];
let mappingDraft = initialMappings();
let dependencyDraft = initialDependencies();
let advancedMode = 'form';
let dependencySequence = 1;
const e = (id) => document.getElementById(id);
const isObject = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);
const ownKeys = (v, allowed) => Object.keys(v).every((key) => allowed.includes(key));
const inputVariables = ['database_name', 'admin_username', 'database', 'username'];
const outputVariables = ['endpoint', 'address', 'port', 'database_name', 'admin_username', 'password'];
const typeInputs = ['database', 'username'];
const typeOutputs = ['host', 'port', 'database', 'username', 'password'];
function advancedError(message = '') { e('advanced-error').textContent = message; e('advanced-error').hidden = !message; }
function advancedStatus(message) { e('advanced-status').textContent = message; }
function selectOptions(values, current, used = []) {
  const all = [...new Set([...values, ...(current ? [current] : [])])];
  return '<option value="">Chưa ánh xạ</option>' + all.filter((value) => !used.includes(value) || value === current).map((value) => '<option value="' + safe(value) + '"' + (value === current ? ' selected' : '') + '>' + safe(value) + '</option>').join('');
}
function inputVariableFor(param) { return Object.entries(mappingDraft.input_mapping).find(([, value]) => value.param === param)?.[0] || ''; }
function outputVariableFor(output) { return Object.entries(mappingDraft.output_mapping).find(([, value]) => value === output)?.[0] || ''; }
function updateInput(param, variable) {
  mappingDraft.input_mapping = Object.fromEntries(Object.entries(mappingDraft.input_mapping).filter(([, value]) => value.param !== param));
  if (variable) mappingDraft.input_mapping[variable] = { param };
  renderAdvancedForm(); advancedStatus('Đã cập nhật ánh xạ đầu vào trong bản mẫu.');
}
function updateOutput(output, variable) {
  mappingDraft.output_mapping = Object.fromEntries(Object.entries(mappingDraft.output_mapping).filter(([, value]) => value !== output));
  if (variable) mappingDraft.output_mapping[variable] = output;
  renderAdvancedForm(); advancedStatus('Đã cập nhật ánh xạ đầu ra trong bản mẫu.');
}
function renderAdvancedForm() {
  e('advanced-form').innerHTML = '<section class="advanced-section"><h3>Ánh xạ đầu vào</h3><p class="field-hint">Chọn biến module nhận từng tham số của loại tài nguyên.</p>' + typeInputs.map((param) => '<div class="mapping-row"><div class="mapping-key">' + safe(param) + '<small>Tham số tài nguyên</small></div><span class="mapping-arrow">→</span><select aria-label="Biến module nhận ' + safe(param) + '" onchange="updateInput(\'' + param + '\',this.value)">' + selectOptions(inputVariables, inputVariableFor(param), Object.keys(mappingDraft.input_mapping)) + '</select></div>').join('') + '</section>' +
    '<section class="advanced-section"><h3>Ánh xạ đầu ra</h3><p class="field-hint">Chọn output module cung cấp giá trị cho tài nguyên.</p>' + typeOutputs.map((output) => '<div class="mapping-row"><div class="mapping-key">' + safe(output) + '<small>' + (output === 'password' ? 'Giá trị bí mật' : 'Output tài nguyên') + '</small></div><span class="mapping-arrow">←</span><select aria-label="Output module cung cấp ' + output + '" onchange="updateOutput(\'' + output + '\',this.value)">' + selectOptions(outputVariables, outputVariableFor(output), Object.keys(mappingDraft.output_mapping)) + '</select></div>').join('') + '</section>' +
    '<section class="advanced-section"><h3>Provider</h3><p class="field-hint">Nguồn truy cập hạ tầng của module.</p><div class="provider-note"><strong>AWS</strong> → Tài khoản của cấu hình<br><span class="field-hint">Tài khoản và vùng được chọn ở phần “Tài khoản & thông số”.</span></div></section>' +
    '<section class="advanced-section"><h3>Tài nguyên đi kèm</h3><p class="field-hint">Khai báo thêm tài nguyên cần tạo cùng cấu hình này.</p>' + (dependencyDraft.length ? dependencyDraft.map((row, index) => '<div class="dependency"><div class="dependency-head"><strong>Tài nguyên ' + (index + 1) + '</strong><button type="button" class="quiet" aria-label="Xóa tài nguyên ' + (index + 1) + '" onclick="removeDependency(' + index + ')">Xóa</button></div><label>Định danh tài nguyên<input class="dependency-descriptor" aria-label="Định danh tài nguyên ' + (index + 1) + '" value="' + safe(row.descriptor) + '" placeholder="vpc.default#@infra" spellcheck="false" oninput="setDependency(' + index + ',\'descriptor\',this.value)"></label><p class="field-hint">Dạng: loại.class#id</p><label>Tham số (JSON)<textarea aria-label="Tham số tài nguyên ' + (index + 1) + '" rows="3" spellcheck="false" oninput="setDependency(' + index + ',\'params\',this.value)">' + safe(row.params) + '</textarea></label><label class="checkline"><input type="checkbox" aria-label="is_dependent tài nguyên ' + (index + 1) + '" ' + (row.is_dependent ? 'checked' : '') + ' onchange="setDependency(' + index + ',\'is_dependent\',this.checked)"><span>Phụ thuộc vào tài nguyên cha<small><code>is_dependent</code> · Tài nguyên cha được lập kế hoạch trước tài nguyên đi kèm.</small></span></label><label class="checkline"><input type="checkbox" aria-label="match_dependents tài nguyên ' + (index + 1) + '" ' + (row.match_dependents ? 'checked' : '') + ' onchange="setDependency(' + index + ',\'match_dependents\',this.checked)"><span>Nối các consumer của tài nguyên cha<small><code>match_dependents</code> · Những bên dùng tài nguyên cha cũng phụ thuộc vào tài nguyên đi kèm.</small></span></label></div>').join('') : '<p class="dependency-empty">Chưa có tài nguyên đi kèm.</p>') + '<button type="button" onclick="addDependency()">+ Thêm tài nguyên đi kèm</button></section>';
}
function setDependency(index, key, value) { dependencyDraft[index][key] = value; advancedError(); advancedStatus('Đã chỉnh tài nguyên đi kèm trong bản mẫu.'); }
function addDependency() { dependencySequence++; dependencyDraft.push({ descriptor: 'redis.default#cache-' + dependencySequence, params: '{}', is_dependent: false, match_dependents: false }); renderAdvancedForm(); advancedStatus('Đã thêm tài nguyên đi kèm trong bản mẫu.'); }
function removeDependency(index) { dependencyDraft.splice(index, 1); renderAdvancedForm(); advancedStatus('Đã xóa tài nguyên đi kèm trong bản mẫu.'); }
function serializeAdvanced() {
  const entries = [];
  for (const [index, row] of dependencyDraft.entries()) {
    if (!row.descriptor.trim()) throw new Error('Nhập định danh cho tài nguyên ' + (index + 1) + '.');
    if (entries.some(([key]) => key === row.descriptor)) throw new Error('Các tài nguyên đi kèm phải có định danh khác nhau.');
    let params;
    try { params = JSON.parse(row.params); } catch { throw new Error('Tham số tài nguyên ' + (index + 1) + ' chưa phải JSON hợp lệ. Dữ liệu đang nhập được giữ lại.'); }
    if (!isObject(params)) throw new Error('Tham số tài nguyên ' + (index + 1) + ' phải là JSON object.');
    entries.push([row.descriptor, { params, is_dependent: row.is_dependent, match_dependents: row.match_dependents }]);
  }
  return { ...structuredClone(mappingDraft), provision: Object.fromEntries(entries) };
}
function readAdvancedJSON() {
  let value;
  try { value = JSON.parse(e('advanced-json-input').value); } catch { throw new Error('JSON chưa hợp lệ. Hãy kiểm tra dấu ngoặc, dấu phẩy và dấu nháy. Nội dung đang nhập được giữ nguyên.'); }
  if (!isObject(value) || !ownKeys(value, ['input_mapping', 'output_mapping', 'provider_bindings', 'provision'])) throw new Error('Bản mẫu hỗ trợ input_mapping, output_mapping, provider_bindings và provision. Giữ nội dung JSON để bạn kiểm tra.');
  for (const key of ['input_mapping', 'output_mapping', 'provider_bindings', 'provision']) if (!isObject(value[key])) throw new Error(key + ' phải là JSON object.');
  const params = [];
  for (const item of Object.values(value.input_mapping)) {
    if (!isObject(item) || !ownKeys(item, ['param']) || !typeInputs.includes(item.param) || params.includes(item.param)) throw new Error('Ánh xạ đầu vào cần có dạng {"biến_module":{"param":"database hoặc username"}}, mỗi tham số một lần.');
    params.push(item.param);
  }
  const outputs = Object.values(value.output_mapping);
  if (outputs.some((item) => !typeOutputs.includes(item)) || new Set(outputs).size !== outputs.length) throw new Error('Output đích cần thuộc loại tài nguyên và không ánh xạ trùng.');
  if (Object.keys(value.provider_bindings).length !== 1 || !isObject(value.provider_bindings.aws) || !ownKeys(value.provider_bindings.aws, ['target']) || value.provider_bindings.aws.target !== 'account') throw new Error('Module minh họa dùng provider AWS với {"target":"account"}.');
  for (const [descriptor, rule] of Object.entries(value.provision)) {
    if (!descriptor.trim() || !isObject(rule) || !ownKeys(rule, ['params', 'is_dependent', 'match_dependents']) || !isObject(rule.params) || typeof rule.is_dependent !== 'boolean' || typeof rule.match_dependents !== 'boolean') throw new Error('Mỗi tài nguyên đi kèm cần params là object và hai cờ is_dependent/match_dependents là boolean. JSON được giữ nguyên.');
  }
  return value;
}
function applyAdvancedJSON() {
  try {
    const value = readAdvancedJSON();
    mappingDraft = { input_mapping: value.input_mapping, output_mapping: value.output_mapping, provider_bindings: value.provider_bindings };
    dependencyDraft = Object.entries(value.provision).map(([descriptor, rule]) => ({ descriptor, params: JSON.stringify(rule.params, null, 2), is_dependent: rule.is_dependent, match_dependents: rule.match_dependents }));
    advancedError(); renderAdvancedForm(); advancedStatus('Đã áp dụng JSON vào bản mẫu. Bạn có thể quay lại biểu mẫu để xem.'); return true;
  } catch (error) { advancedError(error.message); return false; }
}
function setAdvancedMode(mode) {
  if (mode === advancedMode) return;
  if (mode === 'json') {
    try { e('advanced-json-input').value = JSON.stringify(serializeAdvanced(), null, 2); } catch (error) { advancedError(error.message); return; }
  } else if (!applyAdvancedJSON()) return;
  advancedMode = mode; advancedError();
  e('advanced-form').hidden = mode !== 'form'; e('advanced-json').hidden = mode !== 'json';
  e('advanced-form-button').classList.toggle('selected', mode === 'form'); e('advanced-json-button').classList.toggle('selected', mode === 'json');
}
function resetAdvanced() {
  mappingDraft = initialMappings(); dependencyDraft = initialDependencies(); advancedMode = 'form';
  e('advanced-form').hidden = false; e('advanced-json').hidden = true;
  e('advanced-form-button').classList.add('selected'); e('advanced-json-button').classList.remove('selected');
  renderAdvancedForm(); advancedError(); advancedStatus('Đã đặt lại thiết lập nâng cao minh họa.');
}
const resetOriginalPreview = window.resetPreview;
window.resetPreview = function () { resetOriginalPreview(); resetAdvanced(); };
renderAdvancedForm();
if (location.hash === '#advanced') { e('advanced-settings').open = true; e('advanced-settings').scrollIntoView({ block: 'start' }); }
