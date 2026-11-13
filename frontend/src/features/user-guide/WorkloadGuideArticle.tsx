import { href, navigate } from '../../app/routes';
import { t } from '../../shared/i18n';
import { GuideSection, GuideSteps } from './GuideContent';

export const workloadSections = ['overview', 'create', 'containers', 'defaults', 'network', 'dependencies', 'runtime', 'storage', 'schedule', 'apply', 'example', 'problems'] as const;
export const workloadSectionKey = (id: string) => `guide.workload.${id}.title`;
export const workloadSectionId = (id: string) => `workload-${id}`;
const key = (name: string) => `guide.workload.${name}`;

export function WorkloadGuideArticle() {
  return <>
    <h2 id="workloads" tabIndex={-1}>{t('guide.workload.title')}</h2>
    <p>{t(key('scope'))}</p>
    <GuideSection id="workload-overview" titleKey={workloadSectionKey('overview')}>
      <div className="guide-table-scroll" role="region" aria-label={t(key('overview.title'))} tabIndex={0}>
        <table><caption>{t(key('overview.caption'))}</caption><thead><tr><th scope="col">{t(key('overview.kind'))}</th><th scope="col">{t(key('overview.use'))}</th></tr></thead>
          <tbody>{['Deployment', 'StatefulSet', 'CronJob'].map(kind => <tr key={kind}><th scope="row"><code>{kind}</code></th><td>{t(key(`overview.${kind}`))}</td></tr>)}</tbody>
        </table>
      </div>
    </GuideSection>
    <GuideSection id="workload-create" titleKey={workloadSectionKey('create')}><GuideSteps prefix={key('create')} count={5} /><p>{t(key('create.naming'))}</p></GuideSection>
    <GuideSection id="workload-containers" titleKey={workloadSectionKey('containers')}>
      <p>{t(key('containers.image'))}</p><p>{t(key('containers.roles'))}</p><p>{t(key('containers.environment'))}</p>
      <a href={href({ name: 'user-guide' })} onClick={event => { event.preventDefault(); navigate({ name: 'user-guide' }); }}>{t('guide.environment.title')}</a>
    </GuideSection>
    <GuideSection id="workload-defaults" titleKey={workloadSectionKey('defaults')}><p>{t(key('defaults.inherit'))}</p><p>{t(key('defaults.explicit'))}</p><p>{t(key('defaults.pin'))}</p></GuideSection>
    <GuideSection id="workload-network" titleKey={workloadSectionKey('network')}>
      <GuideSteps prefix={key('network')} count={4} /><p>{t(key('network.endpoint'))}</p><p>{t(key('network.headless'))}</p>
    </GuideSection>
    <GuideSection id="workload-dependencies" titleKey={workloadSectionKey('dependencies')}><GuideSteps prefix={key('dependencies')} count={3} /><p>{t(key('dependencies.scope'))}</p></GuideSection>
    <GuideSection id="workload-runtime" titleKey={workloadSectionKey('runtime')}>
      <p>{t(key('runtime.open'))}</p><ul>{['compute', 'probes', 'command', 'scaling', 'security'].map(item => <li key={item}>{t(key(`runtime.${item}`))}</li>)}</ul>
    </GuideSection>
    <GuideSection id="workload-storage" titleKey={workloadSectionKey('storage')}>
      <GuideSteps prefix={key('storage')} count={3} /><ul>{['temporary', 'persistent', 'files', 'host'].map(item => <li key={item}>{t(key(`storage.${item}`))}</li>)}</ul><p>{t(key('storage.retention'))}</p>
    </GuideSection>
    <GuideSection id="workload-schedule" titleKey={workloadSectionKey('schedule')}><GuideSteps prefix={key('schedule')} count={3} /><p>{t(key('schedule.policies'))}</p></GuideSection>
    <GuideSection id="workload-apply" titleKey={workloadSectionKey('apply')}>
      <GuideSteps prefix={key('apply')} count={4} /><p>{t(key('apply.update'))}</p><p>{t(key('apply.remove'))}</p><p>{t(key('apply.resources'))}</p>
    </GuideSection>
    <GuideSection id="workload-example" titleKey={workloadSectionKey('example')}>
      <p>{t(key('example.intro'))}</p>
      <div className="guide-table-scroll" role="region" aria-label={t(key('example.title'))} tabIndex={0}>
        <table><caption>{t(key('example.caption'))}</caption><thead><tr><th scope="col">{t(key('example.field'))}</th><th scope="col">{t(key('example.value'))}</th></tr></thead>
          <tbody>{['template', 'name', 'container', 'image', 'containerPort', 'service', 'target', 'probe'].map(field => <tr key={field}><th scope="row">{t(key(`example.${field}.label`))}</th><td><code>{t(key(`example.${field}.value`))}</code></td></tr>)}</tbody>
        </table>
      </div>
      <p>{t(key('example.finish'))}</p>
    </GuideSection>
    <GuideSection id="workload-problems" titleKey={workloadSectionKey('problems')}>
      <dl>{['image', 'defaults', 'service', 'deploy', 'stale'].map(item => <div key={item}><dt>{t(key(`problems.${item}.title`))}</dt><dd>{t(key(`problems.${item}.body`))}</dd></div>)}</dl>
    </GuideSection>
  </>;
}
