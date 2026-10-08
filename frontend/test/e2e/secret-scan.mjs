// Secret-exposure scan for recorded browser flows.
//
// Why not `page.content().includes(secret)`: React keeps the `value` ATTRIBUTE of
// a controlled <input> in sync with what was typed or pasted, so serialized HTML
// of a form that is being filled in always contains the active password field's
// value. That assertion therefore cannot tell the deliberately typed, masked
// field apart from a real leak. This scan inspects the document structurally:
//   * visible/body text, document title, URL, cookies and web storage;
//   * every attribute of every element;
//   * the `.value` of every input, textarea and select;
// and exempts ONLY the `value` attribute/property of the named active
// <input type="password"> elements. A non-password element matching the
// allowance gets no exemption. Reports name locations, never the secret.

// Self-contained on purpose: Playwright serializes this function into the page.
/* global document -- the default document is the page's own when serialized into the browser. */
export function scanDocument({ values, allow = [] }, doc = document) {
  const needles = values.filter((value) => typeof value === 'string' && value.length >= 8);
  const hit = (text) => needles.length > 0 && typeof text === 'string' && needles.some((needle) => text.includes(needle));
  const leaks = [];
  const add = (where) => { if (!leaks.includes(where)) leaks.push(where); };
  const allowed = new Set();
  for (const selector of allow) {
    for (const element of doc.querySelectorAll(selector)) {
      if (element.tagName === 'INPUT' && element.type === 'password') allowed.add(element);
    }
  }
  const name = (element) => `<${element.tagName.toLowerCase()}${element.id ? `#${element.id}` : ''}>`;
  const body = doc.body;
  if (hit(body.innerText) || hit(body.textContent)) add('page text');
  if (hit(doc.title)) add('document title');
  const view = doc.defaultView;
  if (view) {
    if (hit(view.location.href)) add('location');
    try { if (hit(doc.cookie)) add('cookie'); } catch { /* unreadable cookies hold nothing to leak here */ }
    for (const storage of ['localStorage', 'sessionStorage']) {
      try {
        const area = view[storage];
        for (let index = 0; index < area.length; index += 1) {
          const key = area.key(index);
          if (hit(key) || hit(area.getItem(key))) add(storage);
        }
      } catch { /* storage may be blocked */ }
    }
  }
  for (const element of doc.querySelectorAll('*')) {
    for (const attribute of element.attributes) {
      if (hit(attribute.value) && !(allowed.has(element) && attribute.name === 'value')) add(`attribute ${attribute.name} of ${name(element)}`);
    }
    if (['INPUT', 'TEXTAREA', 'SELECT'].includes(element.tagName) && !allowed.has(element) && hit(element.value)) add(`field value of ${name(element)}`);
  }
  return { leaks, allowedFields: allowed.size };
}

// Throws when a secret is outside the allowed active password field(s). With no
// `allow`, nothing is exempt: use that after submission, when the field must
// have been cleared.
export async function assertNoSecretLeaks(page, values, { allow = [] } = {}) {
  const { leaks } = await page.evaluate(scanDocument, { values, allow });
  if (leaks.length) throw new Error(`secret material found outside the permitted field: ${leaks.join('; ')}`);
}
