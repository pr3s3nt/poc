---
id: ADR-020
artifact: architecture-decision
status: current
last_reviewed: 2026-10-24
---

# ADR-020 — Runtime message catalogs for the Web Console

## Decision and scope

User decision 2026-10-24: the Web Console supports Vietnamese and English, and
adding another language must not require a code change. This supersedes the
VI-01 rule “hard-coded Vietnamese, no i18n, no language selector”. Vietnamese
stays the default and fallback language and the canonical source of Console
labels ([glossary](../../GLOSSARY.md#nhãn-web-console-vi-01)).

## Design

- Console text lives in flat JSON catalogs under `frontend/public/locales/`:
  `<code>.json` maps a stable key (`<feature>.<file>.<message>`) to a message with
  `{name}` placeholders. `manifest.json` lists the selectable languages
  (`code`, `name`, optional `intl` BCP 47 tag for date/number formatting).
- Catalogs are static files of the bundle, fetched at startup. Adding a language
  means adding `<code>.json` and one manifest entry; no source or dependency
  change. Without a rebuild the files can also be dropped into `dist/locales/`.
- The fallback catalog `vi.json` fills keys a translation lacks; a key missing
  everywhere renders as the key, never as blank text.
- Language choice: stored choice (`localStorage`), then browser language, then
  Vietnamese. Changing language persists the choice and reloads the page, so
  module-level labels and formats are rebuilt consistently. Unavailable storage
  limits the choice to the current load; it never blocks the Console.
- No i18n library or runtime dependency is added; `src/shared/i18n` provides
  `t(key, params)`, plural selection through `Intl.PluralRules` (`key_one`,
  `key_other`) and the active `Intl` locale.

## Unchanged boundaries

User-entered names, technical IDs, enums/payloads, `staging`/`production`,
images, descriptors, modules, keys, JSON, placeholders and URLs are not
translated. Backend error sentences are still not translated by regex; the
Console maps stable error codes (T20) to catalog messages. Tests render the
Vietnamese catalog, so Vietnamese locators remain valid.

## Consequences

Every new or changed Console text adds a key to every catalog; a unit test fails
when a catalog misses a key of `vi.json` or changes its placeholders. Wording
changes edit catalogs, not components.
