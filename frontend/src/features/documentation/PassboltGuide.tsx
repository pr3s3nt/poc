import { t } from '../../shared/i18n';
import { ArticleSection } from './ArticleSection';
import score from './examples/passbolt.score.yaml?raw';
import scoreURL from './examples/passbolt.score.yaml?url';
import extension from './examples/passbolt.extension.yaml?raw';
import extensionURL from './examples/passbolt.extension.yaml?url';

export function PassboltGuide() {
  return <>
    <ArticleSection id="passbolt-prepare" title={t('documentation.passbolt.prepare')}>
      <p>{t('documentation.passbolt.scope')}</p>
      <ul>
        <li>{t('documentation.passbolt.delivery')}</li>
        <li>{t('documentation.passbolt.catalog')}</li>
        <li>{t('documentation.passbolt.vault')}</li>
      </ul>
    </ArticleSection>
    <ArticleSection id="passbolt-app" title={t('documentation.passbolt.app')}>
      <p>{t('documentation.passbolt.createApp')}</p>
      <p>{t('documentation.passbolt.secret')}</p>
    </ArticleSection>
    <ArticleSection id="passbolt-files" title={t('documentation.passbolt.files')}>
      <p>{t('documentation.passbolt.uniqueEndpoint')}</p>
      <ul>
        <li><a href={scoreURL} download="passbolt.score.yaml">passbolt.score.yaml</a></li>
        <li><a href={extensionURL} download="passbolt.extension.yaml">passbolt.extension.yaml</a></li>
      </ul>
      <details><summary>passbolt.score.yaml</summary><pre><code>{score}</code></pre></details>
      <details><summary>passbolt.extension.yaml</summary><pre><code>{extension}</code></pre></details>
      <ol>
        <li>{t('documentation.passbolt.openEditor')}</li>
        <li>{t('documentation.passbolt.importFiles')}</li>
        <li>{t('documentation.passbolt.save')}</li>
      </ol>
      <p>{t('documentation.passbolt.storage')}</p>
    </ArticleSection>
    <ArticleSection id="passbolt-deploy" title={t('documentation.passbolt.deploy')}>
      <ol>
        <li>{t('documentation.passbolt.preview')}</li>
        <li>{t('documentation.passbolt.confirm')}</li>
      </ol>
      <p>{t('documentation.passbolt.previewError')}</p>
    </ArticleSection>
    <ArticleSection id="passbolt-verify" title={t('documentation.passbolt.verify')}>
      <ol>
        <li>{t('documentation.passbolt.ready')}</li>
        <li>{t('documentation.passbolt.login')}</li>
        <li>{t('documentation.passbolt.discovery')}</li>
      </ol>
      <pre><code>{"su -m -s /bin/sh -c '/usr/share/php/passbolt/bin/cake passbolt register_user -u EMAIL -f FIRST_NAME -l LAST_NAME -r admin' www-data"}</code></pre>
      <p>{t('documentation.passbolt.bootstrap')}</p>
      <p><a href="https://www.passbolt.com/docs/hosting/install/ce/docker/">{t('documentation.passbolt.upstream')}</a></p>
    </ArticleSection>
  </>;
}
