import type { Workload, ResourceType } from './api';
import type { Dependency } from './DependencyFields';
import type { KeyCatalog, RowContext } from './bindings';

// Another workload can be referenced when it is not this one, not pending deletion and exposes a Service port.
export const serviceCandidates = (workloads: Workload[], self: string): Workload[] =>
  workloads.filter((item) => item.id !== self && item.state !== 'PENDING_DELETE' && item.servicePorts?.length);

export function buildRowContext(catalog: KeyCatalog, dependencies: Dependency[], types: ResourceType[], workloads: Workload[], self: string): RowContext {
  return {
    catalog,
    dependencies: dependencies.filter((dependency) => dependency.alias),
    outputs: (alias) => {
      const type = types.find((item) => item.key === dependencies.find((dependency) => dependency.alias === alias)?.type);
      return type?.outputs.map((output) => output.name);
    },
    services: serviceCandidates(workloads, self).map((item) => ({ id: item.id, ports: item.servicePorts ?? [] })),
  };
}
