import type { ComponentType } from 'react';
import { documentationArticles, href, type DocumentationArticle } from '../../app/routes';
import { t } from '../../shared/i18n';
import { CreateApplicationGuide } from './CreateApplicationGuide';
import './documentation.css';

const articles: Record<DocumentationArticle, { titleKey: string; Content: ComponentType }> = {
  'create-application': {
    titleKey: 'applications.createApplication.createApplication',
    Content: CreateApplicationGuide,
  },
};

export function DocumentationPage({ article }: { article: DocumentationArticle }) {
  const { titleKey, Content } = articles[article];
  return <div className="page documentation-layout">
    <nav className="content-panel documentation-menu" aria-labelledby="documentation-articles">
      <h2 id="documentation-articles">{t('documentation.articles')}</h2>
      <ul>
        {documentationArticles.map((slug) => <li key={slug}>
          <a href={href({ name: 'documentation', article: slug })} aria-current={article === slug ? 'page' : undefined}>
            {t(articles[slug].titleKey)}
          </a>
        </li>)}
      </ul>
    </nav>
    <article aria-labelledby="documentation-title">
      <header className="page-header">
        <div>
          <p className="eyebrow">{t('app.appShell.documentation')}</p>
          <h1 id="documentation-title">{t(titleKey)}</h1>
        </div>
      </header>
      <Content />
    </article>
  </div>;
}
