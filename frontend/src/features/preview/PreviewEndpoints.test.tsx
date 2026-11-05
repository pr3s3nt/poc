import { render, screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { PreviewEndpoints } from './PreviewEndpoints';
import type { PendingChange } from '../workloads/api';

const change = (workloadId: string, url?: string): PendingChange => ({ workloadId, action: 'UPDATE', delta: {}, resources: { existing: [], new: [], unreferenced: [] }, planHash: 'h', ...(url ? { endpoint: { hostname: new URL(url).hostname, url } } : {}) });

describe('PreviewEndpoints', () => {
  it('lists each prospective public address once', () => {
    render(<PreviewEndpoints changes={[change('frontend', 'https://shop.apps.example.com/'), change('backend')]} />);
    const panel = screen.getByRole('status', { name: 'Địa chỉ public' });
    expect(within(panel).getAllByRole('listitem')).toHaveLength(1);
    expect(within(panel).getByText('https://shop.apps.example.com/')).toBeInTheDocument();
  });

  it('renders nothing without endpoints', () => {
    const { container } = render(<PreviewEndpoints changes={[change('backend')]} />);
    expect(container).toBeEmptyDOMElement();
  });
});
