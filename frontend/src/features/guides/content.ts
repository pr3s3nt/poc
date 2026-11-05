import acceptanceApp from '../../../../orchestrator_docs/guides/dev/acceptance-app.md?raw';

export type GuideAudience = 'developer' | 'platform';

export function canReadGuide(role: string, audience: GuideAudience): boolean {
  return role === 'ADMIN' || role === 'PLATFORM_ENGINEER' || (role === 'DEVELOPER' && audience === 'developer');
}

// The repository guide remains the source. Maintainer references and metadata
// stay in the repository; published illustrations are served by this Console.
export const developerGuide = acceptanceApp
  .replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n/, '')
  .split('\n## Nguồn đối chiếu và phạm vi xác minh')[0]!
  .replaceAll('../../../frontend/public/', import.meta.env.BASE_URL)
  .trim();

export function headingId(text: string): string {
  return text.normalize('NFD').replace(/[\u0300-\u036f]/g, '').replace(/đ/g, 'd').replace(/Đ/g, 'D')
    .toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
}

export const guideSections = [...developerGuide.matchAll(/^## (.+)$/gm)]
  .map((match) => ({ title: match[1]!, id: headingId(match[1]!) }));
