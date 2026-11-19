import { href } from '../../app/routes';
import { t } from '../../shared/i18n';

export function CreateApplicationGuide() {
  return <article className="page narrow-page">
    <header className="page-header">
      <div>
        <p className="eyebrow">{t('app.appShell.documentation')}</p>
        <h1>{t('applications.createApplication.createApplication')}</h1>
      </div>
    </header>
    <section className="content-panel" aria-labelledby="guide-create-steps">
      <div className="section-header"><h2 id="guide-create-steps">{t('documentation.createApplication.steps')}</h2></div>
      <ol>
        <li>{t('documentation.createApplication.openForm')}</li>
        <li>{t('documentation.createApplication.enterName')}</li>
        <li>{t('documentation.createApplication.submit')}</li>
      </ol>
      <p>{t('documentation.createApplication.duplicateName')}</p>
      <a href={href({ name: 'create-application' })}>{t('documentation.createApplication.formLink')}</a>
    </section>
    <section className="content-panel" aria-labelledby="guide-next-steps">
      <div className="section-header"><h2 id="guide-next-steps">{t('documentation.createApplication.nextSteps')}</h2></div>
      <p>{t('documentation.createApplication.environments')}</p>
      <ol>
        <li>{t('documentation.createApplication.checkSettings')}</li>
        <li>{t('documentation.createApplication.addWorkload')}</li>
        <li>{t('documentation.createApplication.reviewAndDeploy')}</li>
      </ol>
    </section>
  </article>;
}
