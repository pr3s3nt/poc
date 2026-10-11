import { parseAllDocuments } from 'yaml';

export type ParsedScore = { ok: true; score: Record<string, unknown> } | { ok: false; error: string };

// YAML anchors may be reused a few times; more is treated as an alias bomb.
const MAX_ALIASES = 20;

// Parses one Score document locally. YAML is a superset of JSON, so both are
// accepted; nothing is sent to the server and the text is never rewritten.
// The result is a plain JSON value, so it can always be serialized.
export function parseScoreText(text: string): ParsedScore {
  if (!text.trim()) return { ok: false, error: 'là bắt buộc.' };
  let documents;
  try { documents = parseAllDocuments(text, { prettyErrors: false }); }
  catch { return { ok: false, error: 'không phải YAML hoặc JSON hợp lệ.' }; }
  if (!Array.isArray(documents)) return { ok: false, error: 'không phải YAML hoặc JSON hợp lệ.' };
  if (documents.length !== 1) return { ok: false, error: 'phải chứa đúng một tài liệu.' };
  const [document] = documents;
  if (!document || document.errors.length) return { ok: false, error: 'không phải YAML hoặc JSON hợp lệ.' };
  let value: unknown;
  try { value = document.toJS({ maxAliasCount: MAX_ALIASES }); }
  catch { return { ok: false, error: `dùng quá nhiều alias YAML hoặc alias đệ quy; hãy viết giá trị ra đầy đủ (tối đa ${MAX_ALIASES} lần dùng alias).` }; }
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return { ok: false, error: 'phải là một đối tượng Score.' };
  try { return { ok: true, score: JSON.parse(JSON.stringify(value)) as Record<string, unknown> }; }
  catch { return { ok: false, error: 'phải là dữ liệu JSON thuần, không tự tham chiếu.' }; }
}
