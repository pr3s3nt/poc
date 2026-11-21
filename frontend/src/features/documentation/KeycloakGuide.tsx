import { t } from '../../shared/i18n';
import { ArticleSection } from './ArticleSection';
import score from './examples/keycloak.score.yaml?raw';
import scoreURL from './examples/keycloak.score.yaml?url';
import extension from './examples/keycloak.extension.yaml?raw';
import extensionURL from './examples/keycloak.extension.yaml?url';

export function KeycloakGuide() {
  return <>
    <ArticleSection id="keycloak-prepare" title={t('documentation.keycloak.prepare')}>
      <p>{t('documentation.keycloak.scope')}</p>
      <ul>
        <li>{t('documentation.keycloak.delivery')}</li>
        <li>{t('documentation.keycloak.catalog')}</li>
        <li>{t('documentation.keycloak.vault')}</li>
      </ul>
    </ArticleSection>
    <ArticleSection id="keycloak-app" title={t('documentation.keycloak.app')}>
      <p>{t('documentation.keycloak.createApp')}</p>
      <p>{t('documentation.keycloak.secret')}</p>
    </ArticleSection>
    <ArticleSection id="keycloak-files" title={t('documentation.keycloak.files')}>
      <p>{t('documentation.keycloak.uniqueEndpoint')}</p>
      <ul>
        <li><a href={scoreURL} download="keycloak.score.yaml">keycloak.score.yaml</a></li>
        <li><a href={extensionURL} download="keycloak.extension.yaml">keycloak.extension.yaml</a></li>
      </ul>
      <details><summary>keycloak.score.yaml</summary><pre><code>{score}</code></pre></details>
      <details><summary>keycloak.extension.yaml</summary><pre><code>{extension}</code></pre></details>
      <ol>
        <li>{t('documentation.keycloak.openEditor')}</li>
        <li>{t('documentation.keycloak.importFiles')}</li>
        <li>{t('documentation.keycloak.save')}</li>
      </ol>
    </ArticleSection>
    <ArticleSection id="keycloak-deploy" title={t('documentation.keycloak.deploy')}>
      <ol>
        <li>{t('documentation.keycloak.preview')}</li>
        <li>{t('documentation.keycloak.confirm')}</li>
      </ol>
      <p>{t('documentation.keycloak.previewError')}</p>
    </ArticleSection>
    <ArticleSection id="keycloak-verify" title={t('documentation.keycloak.verify')}>
      <ol>
        <li>{t('documentation.keycloak.ready')}</li>
        <li>{t('documentation.keycloak.login')}</li>
        <li>{t('documentation.keycloak.discovery')}</li>
      </ol>
      <p>{t('documentation.keycloak.bootstrap')}</p>
      <p><a href="https://www.keycloak.org/server/containers">{t('documentation.keycloak.upstream')}</a></p>
    </ArticleSection>
  </>;
}
