import type { Application, EnvironmentKey } from '../types/application';

export function ApplicationTarget({ environment }: { application: Application; environment: EnvironmentKey }) {
  return <p className="application-target" aria-label="Đích triển khai">{environment} · Đích được xác định từ Resource Definition và các tham chiếu Workload.</p>;
}
