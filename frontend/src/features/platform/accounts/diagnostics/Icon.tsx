import type { ReactNode } from 'react';

type Name = 'diagnose' | 'play' | 'settings' | 'refresh' | 'arrow' | 'search' | 'clock' | 'download' | 'cluster' | 'cloud';

const paths: Record<Name, ReactNode> = {
  diagnose: <><path d="M4 4h16v16H4zM8 2v4m8-4v4M8 18v4m8-4v4M2 8h4m12 0h4M2 16h4m12 0h4" /><path d="m7 12 3 0 2-4 2 8 2-4h2" /></>,
  play: <path d="m9 5 11 7-11 7V5Z" />,
  settings: <><path d="M4 6h16M4 12h16M4 18h16" /><circle cx="9" cy="6" r="2" /><circle cx="15" cy="12" r="2" /><circle cx="9" cy="18" r="2" /></>,
  refresh: <path d="M20 11a8 8 0 1 0-2 6M20 4v7h-7" />,
  arrow: <path d="m9 5 7 7-7 7" />,
  search: <><circle cx="10" cy="10" r="6" /><path d="m15 15 5 5" /></>,
  clock: <><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></>,
  download: <path d="M12 3v12m-4-4 4 4 4-4M4 16v5h16v-5" />,
  cluster: <><rect x="8" y="2" width="8" height="6" rx="1" /><rect x="2" y="16" width="8" height="6" rx="1" /><rect x="14" y="16" width="8" height="6" rx="1" /><path d="M12 8v4M6 16v-4h12v4" /></>,
  cloud: <path d="M6 18a5 5 0 0 1-1-10 7 7 0 0 1 13 0 5 5 0 0 1 0 10H6Z" />,
};

export function Icon({ name, size = 18 }: { name: Name; size?: number }) {
  return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{paths[name]}</svg>;
}
