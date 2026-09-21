import type { ReactNode } from 'react';

interface CalloutProps {
  readonly tone: 'error' | 'info' | 'success';
  readonly title: string;
  readonly children?: ReactNode;
}

export function Callout({ tone, title, children }: CalloutProps) {
  return (
    <div className={`callout callout-${tone}`} role={tone === 'error' ? 'alert' : 'status'}>
      <strong>{title}</strong>
      {children ? <div className="callout-body">{children}</div> : null}
    </div>
  );
}
