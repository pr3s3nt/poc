import { parseAllDocuments } from 'yaml';

export type ParsedScore = { ok: true; score: Record<string, unknown> } | { ok: false; error: string };

// YAML anchors may be reused a few times; more is treated as an alias bomb.
const MAX_ALIASES = 20;

// Parses one Score document locally. YAML is a superset of JSON, so both are
// accepted; nothing is sent to the server and the text is never rewritten.
// The result is a plain JSON value, so it can always be serialized.
export function parseScoreText(text: string): ParsedScore {
  if (!text.trim()) return { ok: false, error: 'is required.' };
  let documents;
  try { documents = parseAllDocuments(text, { prettyErrors: false }); }
  catch { return { ok: false, error: 'is not valid YAML or JSON.' }; }
  if (!Array.isArray(documents)) return { ok: false, error: 'is not valid YAML or JSON.' };
  if (documents.length !== 1) return { ok: false, error: 'must contain exactly one document.' };
  const [document] = documents;
  if (!document || document.errors.length) return { ok: false, error: `is not valid YAML or JSON${document?.errors[0] ? `: ${document.errors[0].message.split('\n')[0]}` : '.'}` };
  let value: unknown;
  try { value = document.toJS({ maxAliasCount: MAX_ALIASES }); }
  catch { return { ok: false, error: `uses too many or recursive YAML aliases; write the values out (at most ${MAX_ALIASES} alias uses).` }; }
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return { ok: false, error: 'must be a Score object.' };
  try { return { ok: true, score: JSON.parse(JSON.stringify(value)) as Record<string, unknown> }; }
  catch { return { ok: false, error: 'must be plain JSON data without self-references.' }; }
}
