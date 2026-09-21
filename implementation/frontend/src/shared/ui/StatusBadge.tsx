interface StatusBadgeProps {
  readonly status: string;
}

const TONE: Record<string, string> = {
  SUCCEEDED: 'ok',
  READY: 'ok',
  FAILED: 'bad',
  PLANNING: 'busy',
  PROVISIONING: 'busy',
  DEPLOYING: 'busy',
  APPLYING: 'busy',
};

export function StatusBadge({ status }: StatusBadgeProps) {
  const tone = TONE[status] ?? 'neutral';
  return (
    <span className={`badge badge-${tone}`} data-testid="status-badge">
      {status}
    </span>
  );
}
