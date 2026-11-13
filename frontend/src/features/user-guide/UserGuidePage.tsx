import type { ReactNode } from 'react';
import { t } from '../../shared/i18n';

const sections = ['start', 'table', 'sources', 'envfile', 'syntax', 'secrets', 'import', 'save', 'problems'] as const;
type Section = typeof sections[number];

// Synthetic examples only. These are import grammar, never resolved config values.
export const environmentExample = [
  'APP_ENV=production',
  'LOG_LEVEL=info',
  'PORT=8080',
  'APP_NAME="Demo API"',
  'REGION=@variable.REGION',
  'API_TOKEN=@secret.API_TOKEN',
  'DB_HOST=@db.host',
  'DB_PASSWORD=@db.password',
  'BACKEND_ADDRESS=@service.backend.http',
].join('\n');
export const secretExample = '# @secret\nNEW_API_TOKEN="example-only-replace-me"';
export const syntaxExample = [
  'MESSAGE="  Demo API  "',
  'HASH="abc # 123"',
  'QUERY=a=b=c',
  'OPTIONAL_VALUE=',
  'MULTILINE="line 1\\nline 2"',
  "WINDOWS_PATH='C:\\data\\files'",
  'CONTACT=@@team.example',
].join('\n');

function Steps({ section, count }: { section: Section; count: number }) {
  return <ol>{Array.from({ length: count }, (_, i) => <li key={i}>{t(`guide.${section}.step${i + 1}`)}</li>)}</ol>;
}

function GuideSection({ id, children }: { id: Section; children: ReactNode }) {
  return <section className="guide-section" aria-labelledby={`guide-${id}`}>
    <h3 id={`guide-${id}`} tabIndex={-1}>{t(`guide.${id}.title`)}</h3>{children}
  </section>;
}

export function UserGuidePage() {
  return <section className="page guide-page">
    <header className="page-header"><h1>{t('guide.title')}</h1></header>
    <div className="guide-layout">
      <nav className="content-panel guide-contents" aria-label={t('guide.contents')}>
        <h2>{t('guide.contents')}</h2>
        <a href="#environment-variables">{t('guide.environment.title')}</a>
        <ul>{sections.map(id => <li key={id}><a href={`#guide-${id}`}>{t(`guide.${id}.title`)}</a></li>)}</ul>
      </nav>
      <article className="content-panel guide-article" aria-labelledby="environment-variables">
        <h2 id="environment-variables" tabIndex={-1}>{t('guide.environment.title')}</h2>
        <p>{t('guide.environment.scope')}</p>
        <GuideSection id="start"><Steps section="start" count={3} /></GuideSection>
        <GuideSection id="table"><Steps section="table" count={3} /><p>{t('guide.table.secret')}</p></GuideSection>
        <GuideSection id="sources">
          <div className="guide-table-scroll" role="region" aria-label={t('guide.sources.title')} tabIndex={0}>
            <table><caption>{t('guide.sources.caption')}</caption>
              <thead><tr><th scope="col">{t('guide.sources.value')}</th><th scope="col">{t('guide.sources.meaning')}</th></tr></thead>
              <tbody>{['literal', 'variable', 'secret', 'resource', 'service', 'escape'].map(kind => <tr key={kind}><th scope="row"><code>{t(`guide.sources.${kind}.example`)}</code></th><td>{t(`guide.sources.${kind}.description`)}</td></tr>)}</tbody>
            </table>
          </div>
          <p>{t('guide.sources.requirements')}</p>
        </GuideSection>
        <GuideSection id="envfile"><p>{t('guide.envfile.open')}</p><pre tabIndex={0} aria-label={t('guide.envfile.example')}><code>{environmentExample}</code></pre><p>{t('guide.envfile.metadata')}</p></GuideSection>
        <GuideSection id="syntax">
          <ul>{Array.from({ length: 7 }, (_, i) => <li key={i}>{t(`guide.syntax.rule${i + 1}`)}</li>)}</ul>
          <pre tabIndex={0} aria-label={t('guide.syntax.example')}><code>{syntaxExample}</code></pre>
          <p>{t('guide.syntax.at')}</p><p>{t('guide.syntax.expansion')}</p>
        </GuideSection>
        <GuideSection id="secrets">
          <p>{t('guide.secrets.direct')}</p><pre tabIndex={0} aria-label={t('guide.secrets.example')}><code>{secretExample}</code></pre>
          <Steps section="secrets" count={4} />
          <p>{t('guide.secrets.marker')}</p><p>{t('guide.secrets.boundary')}</p><p>{t('guide.secrets.results')}</p>
        </GuideSection>
        <GuideSection id="import"><Steps section="import" count={3} /><p>{t('guide.import.edit')}</p><p>{t('guide.import.download')}</p></GuideSection>
        <GuideSection id="save"><Steps section="save" count={3} /></GuideSection>
        <GuideSection id="problems">
          <dl>{['name', 'reference', 'quote', 'marker', 'file', 'metadata', 'secret'].map(kind => <div key={kind}><dt>{t(`guide.problems.${kind}.title`)}</dt><dd>{t(`guide.problems.${kind}.body`)}</dd></div>)}</dl>
        </GuideSection>
      </article>
    </div>
  </section>;
}
