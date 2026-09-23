export function Status({ children, tone = 'neutral' }: { children: string; tone?: 'good' | 'neutral' | 'draft' }) {
  return <span className={`status status-${tone}`}><span className="status-dot" />{children}</span>;
}
