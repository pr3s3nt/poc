import { t } from '../../shared/i18n';
import { ArticleSection } from './ArticleSection';

const literalExample = [
  'LOG_LEVEL=info',
  'APP_NAME="Demo API"',
  'PORT=8080',
  'CONTACT=@@team.example',
].join('\n');

const secretExample = '# @secret\nNEW_API_TOKEN=example-only-replace-me';

export function EnvFileGuide() {
  return <>
    <ArticleSection id="env-file-syntax" title={t('documentation.envFile.syntax')}>
      <p>{t('documentation.envFile.format', { format: 'NAME=value' })}</p>
      <pre><code>{literalExample}</code></pre>
      <ul>
        <li>{t('documentation.envFile.nameRule')}</li>
        <li>{t('documentation.envFile.quoteRule')}</li>
        <li>{t('documentation.envFile.literalRule')}</li>
      </ul>
    </ArticleSection>
    <ArticleSection id="env-file-references" title={t('documentation.envFile.references')}>
      <p>{t('documentation.envFile.referenceRule')}</p>
      <ul>
        <li><code>REGION=@variable.REGION</code> {t('documentation.envFile.variableReference')}</li>
        <li><code>API_TOKEN=@secret.API_TOKEN</code> {t('documentation.envFile.secretReference')}</li>
        <li><code>DB_HOST=@db.host</code> {t('documentation.envFile.resourceReference')}</li>
        <li><code>BACKEND_ADDRESS=@service.backend.http</code> {t('documentation.envFile.serviceReference')}</li>
      </ul>
    </ArticleSection>
    <ArticleSection id="env-file-import" title={t('documentation.envFile.import')}>
      <ol>
        <li>{t('documentation.envFile.openEditor')}</li>
        <li>{t('documentation.envFile.selectFile')}</li>
        <li>{t('documentation.envFile.reviewImport')}</li>
        <li>{t('documentation.envFile.reviewDraft')}</li>
      </ol>
      <p>{t('documentation.envFile.download')}</p>
    </ArticleSection>
    <ArticleSection id="env-file-secrets" title={t('documentation.envFile.secrets')}>
      <p>{t('documentation.envFile.secretMarker')}</p>
      <pre><code>{secretExample}</code></pre>
      <p>{t('documentation.envFile.secretConfirm')}</p>
      <p>{t('documentation.envFile.secretBoundary')}</p>
    </ArticleSection>
  </>;
}
