import { href } from '../../app/routes';
import { t } from '../../shared/i18n';
import { ArticleSection } from './ArticleSection';

export function CreateWorkloadGuide() {
  return <>
    <ArticleSection id="workload-add" title={t('workloads.workloadEditor.addWorkload')}>
      <ol>
        <li>{t('documentation.createWorkload.open')}</li>
        <li>{t('documentation.createWorkload.choose')}</li>
        <li>{t('documentation.createWorkload.container')}</li>
      </ol>
      <p>{t('documentation.createWorkload.naming')}</p>
      <p>{t('documentation.createWorkload.example', { workload: 'web', container: 'main', image: 'nginx:alpine' })}</p>
    </ArticleSection>
    <ArticleSection id="workload-configure" title={t('documentation.createWorkload.configure')}>
      <p>{t('documentation.createWorkload.runtime')}</p>
      <ul>
        <li><code>Deployment</code> {t('documentation.createWorkload.deployment')}</li>
        <li><code>StatefulSet</code> {t('documentation.createWorkload.statefulSet')}</li>
        <li><code>CronJob</code> {t('documentation.createWorkload.cronJob')}</li>
      </ul>
      <p>{t('documentation.createWorkload.dependencies')}</p>
      <p>{t('documentation.createWorkload.network')}</p>
      <p>{t('documentation.createWorkload.advanced')}</p>
      <a href={href({ name: 'documentation', article: 'write-env-file' })}>{t('documentation.envFile.title')}</a>
    </ArticleSection>
    <ArticleSection id="workload-deploy" title={t('documentation.createWorkload.deploy')}>
      <ol>
        <li>{t('documentation.createWorkload.saveDraft')}</li>
        <li>{t('documentation.createWorkload.preview')}</li>
        <li>{t('documentation.createWorkload.confirmDeploy')}</li>
      </ol>
    </ArticleSection>
  </>;
}
