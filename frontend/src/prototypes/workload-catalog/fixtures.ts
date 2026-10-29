import { emptyEnvironment, type Application } from '../../shared/types/application';

export const application: Application = {
  id: 'passbolt', name: 'Passbolt', subdomain: 'passbolt',
  environments: { staging: emptyEnvironment, production: emptyEnvironment },
  workloads: { staging: [], production: [] },
};

// Only this standalone entry installs the fixture transport. All API requests
// terminate in memory, including unsupported mutations; none reach a backend.
export function installFixtures() {
  const original = window.fetch.bind(window);
  window.fetch = async (input, init) => {
    const url = new URL(typeof input === 'string' ? input : input instanceof URL ? input.href : input.url, window.location.href);
    if (!url.pathname.startsWith('/api/')) return original(input, init);
    const method = init?.method ?? (input instanceof Request ? input.method : 'GET');
    if (method !== 'GET') return Response.json({ code: 'SERVICE_UNAVAILABLE' }, { status: 503 });
    if (url.pathname.includes('/observability/')) {
      const now = new Date().toISOString();
      return Response.json({state:'NOT_DEPLOYED',from:now,to:now,generatedAt:now,stepSeconds:60,logCount:0,errorCount:0,eventCount:0,signalCount:0,logsRetentionDays:7,metricsRetentionDays:7,sources:[],latest:[],points:[],peaks:[],usagePeaks:[],events:[],histogram:[],workloads:[],logs:[],hasHistory:false});
    }
    if (url.pathname.endsWith('/workloads')) return Response.json({workloads:[],draftVersion:0});
    if (url.pathname.endsWith('/resources')) return Response.json({resources:[]});
    if (url.pathname.endsWith('/deployments')) return Response.json({deployments:[]});
    return Response.json({environment:{key:'staging',...emptyEnvironment}});
  };
}
