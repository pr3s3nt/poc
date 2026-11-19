import type { ReactNode } from 'react';

export function ArticleSection({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return <section className="content-panel" aria-labelledby={id}>
    <div className="section-header"><h2 id={id}>{title}</h2></div>
    {children}
  </section>;
}
