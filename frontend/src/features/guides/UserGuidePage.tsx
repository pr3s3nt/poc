import { createContext, useContext, useState } from 'react';
import Markdown, { type Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { currentLocale, t } from '../../shared/i18n';
import { canReadGuide, developerGuide, guideSections, headingId } from './content';
import './guides.css';

const ImageViewer = createContext<(src: string, caption: string) => void>(() => {});

function GuideImage({ src, alt = '' }: { src?: string; alt?: string }) {
  const onOpen = useContext(ImageViewer);
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);
  if (!src) return null;
  return <span className="guide-illustration">
    {failed ? <span className="guide-image-error" role="status">{t('guides.imageError')} <Button onClick={() => { setFailed(false); setAttempt(value => value + 1); }}>{t('guides.retryImage')}</Button></span>
      : <button className="guide-image-button" type="button" aria-label={t('guides.openImage', { caption: alt })} onClick={() => onOpen(src, alt)}>
        <img key={attempt} src={src} alt={alt} loading="lazy" onError={() => setFailed(true)} />
      </button>}
    <span className="guide-image-caption">{alt}</span>
  </span>;
}

// Stable component identities preserve image-button focus while the viewer opens.
const markdownComponents: Components = {
  h1: ({ children }) => <h2 className="guide-article-title">{children}</h2>,
  h2: ({ children }) => <h2 id={headingId(String(children))}>{children}</h2>,
  table: ({ children }) => <div className="guide-table-scroll" tabIndex={0}><table>{children}</table></div>,
  img: GuideImage,
};

export function UserGuidePage({ role }: { role: string }) {
  const [image, setImage] = useState<{ src: string; caption: string }>();
  return <section className="page user-guide-page">
    <header className="page-header"><h1>{t('guides.title')}</h1></header>
    {!canReadGuide(role, 'developer') ? <div className="empty-state">{t('guides.empty')}</div> : <>
      {currentLocale() !== 'vi' ? <p className="feature-note">{t('guides.vietnameseContent')}</p> : null}
      <nav className="content-panel guide-contents" aria-label={t('guides.contents')}>
        <h2>{t('guides.contents')}</h2>
        <ul>{guideSections.map(section => <li key={section.id}><a href={`#${section.id}`}>{section.title}</a></li>)}</ul>
      </nav>
      <article className="content-panel guide-article" lang="vi">
        <ImageViewer.Provider value={(src, caption) => setImage({ src, caption })}>
          <Markdown remarkPlugins={[remarkGfm]} skipHtml components={markdownComponents}>{developerGuide}</Markdown>
        </ImageViewer.Provider>
      </article>
    </>}
    {image ? <Dialog label={image.caption} onClose={() => setImage(undefined)}>
      <div className="guide-image-dialog"><div className="section-header"><h2>{image.caption}</h2><Button onClick={() => setImage(undefined)}>{t('guides.closeImage')}</Button></div>
        <a href={image.src} target="_blank" rel="noreferrer">{t('guides.openImage', { caption: image.caption })}</a>
        <img src={image.src} alt={image.caption} />
      </div>
    </Dialog> : null}
  </section>;
}
