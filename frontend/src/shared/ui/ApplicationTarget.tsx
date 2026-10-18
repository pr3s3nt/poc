import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../types/application';

// Shows the selected Environment's saved execution target (ADR-011). Each
// Environment has its own; there is no Application-wide target.
const profileLabels: Record<string, string> = { 'internal-k8s': 'Cluster nội bộ', 'aws-eks': 'AWS' };
const profileLabel = (profile: string) => (Object.hasOwn(profileLabels, profile) ? profileLabels[profile] ?? profile : profile);

export function ApplicationTarget({ application, environment }: { application: Application; environment: EnvironmentKey }) {
  const target = application.environments[environment];
  if (!target.configured) {
    // ADR-016: no Environment account; Definitions and references decide the target.
    return <p className="application-target" aria-label="Đích triển khai">{environment} · Đích triển khai do Resource Definition và tham chiếu của Workload quyết định; Môi trường không chọn tài khoản. <a href="#" onClick={(event) => { event.preventDefault(); navigate({ name: 'settings', applicationId: application.id, environment }); }}>Cài đặt môi trường</a></p>;
  }
  return <p className="application-target" aria-label="Đích triển khai">{environment} · Kết nối <strong>{target.connectionName || target.connectionKey}</strong> ({target.connectionKey}) · Phạm vi triển khai <strong>{profileLabel(target.profile)}</strong> ({target.profile}){target.region ? <> · Vùng <strong>{target.region}</strong></> : null}</p>;
}
