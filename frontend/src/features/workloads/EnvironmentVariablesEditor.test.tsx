import { useState } from 'react';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { EnvironmentVariablesEditor } from './EnvironmentVariablesEditor';
import { emptyBinding, type Binding, type RowContext } from './bindings';
import { bothConfigured } from '../../test/targets';
const context: RowContext = { catalog: { status: 'ready', version: 1, keys: [] }, dependencies: [], outputs: () => [], services: [] };
function setup(initial: Binding[] = [{ ...emptyBinding(), name: 'KEEP', value: 'old' }]) {
  const blocked = vi.fn();
  function Host() {
    const [bindings, setBindings] = useState(initial);
    return <EnvironmentVariablesEditor editorId={0} container="main" bindings={bindings} catalog={context.catalog} dependencies={[]} types={[]} workloads={[]} self="test" context={context} disabled={false} showIncomplete={false} onFocused={() => {}} onChange={setBindings} onCreateKey={() => {}} onAddResource={() => {}} application={{ id: 'test', name: 'Test', environments: bothConfigured(), workloads: { staging: [], production: [] } }} environment="staging" onConfiguration={() => {}} onBlocked={blocked} onBusy={() => {}} onReplaceSecret={() => {}} />;
  }
  render(<Host />); return { user: userEvent.setup(), blocked };
}
function file(text: string) { const result = new File([text], '.env', { type: 'text/plain' }); Object.defineProperty(result, 'arrayBuffer', { value: async () => new TextEncoder().encode(text).buffer }); return result; }
it('reviews file collisions with keep default and preserves absent variables', async () => {
  const { user } = setup(); await user.click(screen.getByRole('button', { name: 'Nhập từ file .env' }));
  await user.upload(screen.getByLabelText('Chọn file UTF-8'), file('KEEP=new\nADDED=next'));
  const dialog = within(await screen.findByRole('dialog', { name: 'Rà soát file .env' }));
  expect(dialog.getByLabelText('KEEP')).toHaveValue('keep');
  await user.click(dialog.getByRole('button', { name: 'Nhập vào bản nháp' }));
  expect(screen.getByLabelText('Nội dung .env')).toHaveValue('KEEP="old"\nADDED="next"');
  await user.upload(screen.getByLabelText('Chọn file UTF-8'), file('KEEP=replaced'));
  const second = within(await screen.findByRole('dialog'));
  await user.selectOptions(second.getByLabelText('KEEP'), 'replace');
  await user.click(second.getByRole('button', { name: 'Nhập vào bản nháp' }));
  expect(screen.getByLabelText('Nội dung .env')).toHaveValue('KEEP="replaced"\nADDED="next"');
});
it('retains an invalid file for correction without overwriting current rows', async () => {
  const { user, blocked } = setup(); await user.click(screen.getByRole('button', { name: 'Nhập từ file .env' }));
  await user.upload(screen.getByLabelText('Chọn file UTF-8'), file('NEW=ok\nBROKEN'));
  await waitFor(() => expect(screen.getByLabelText('Nội dung .env')).toHaveValue('NEW=ok\nBROKEN'));
  expect(screen.getByLabelText('Tên biến, dòng 1')).toHaveValue('KEEP');
  expect(blocked).toHaveBeenLastCalledWith(0, true);
  fireEvent.change(screen.getByLabelText('Nội dung .env'), { target: { value: 'NEW=ok' } });
  const dialog = within(await screen.findByRole('dialog'));
  await user.click(dialog.getByRole('button', { name: 'Nhập vào bản nháp' }));
  expect(screen.getByLabelText('Nội dung .env')).toHaveValue('KEEP="old"\nNEW="ok"');
});
it('keeps invalid text when another draft field refreshes bindings', async () => {
  const blocked = vi.fn(), onChange = vi.fn(); const row = { ...emptyBinding(), name: 'OLD', value: 'kept' };
  const props = { editorId: 0, container: 'main', bindings: [row], catalog: context.catalog, dependencies: [], types: [], workloads: [], self: 'test', context, disabled: false, showIncomplete: false, onFocused() {}, onChange, onCreateKey() {}, onAddResource() {}, application: { id: 'test', name: 'Test', environments: bothConfigured(), workloads: { staging: [], production: [] } }, environment: 'staging' as const, onConfiguration() {}, onBlocked: blocked, onBusy() {}, onReplaceSecret() {} };
  const view = render(<EnvironmentVariablesEditor {...props} />); const user = userEvent.setup();
  await user.click(screen.getByRole('button', { name: 'Nhập từ file .env' }));
  fireEvent.change(screen.getByLabelText('Nội dung .env'), { target: { value: 'BROKEN' } });
  view.rerender(<EnvironmentVariablesEditor {...props} bindings={[{ ...row }]} />);
  expect(screen.getByLabelText('Nội dung .env')).toHaveValue('BROKEN');expect(onChange).not.toHaveBeenCalled();
});
it('publishes formerly invalid text atomically when its missing dependency becomes available', async () => {
  const blocked = vi.fn(), onChange = vi.fn(); const row = { ...emptyBinding(), name: 'OLD', value: 'kept' };
  const props = { editorId: 0, container: 'main', bindings: [row], catalog: context.catalog, dependencies: [], types: [], workloads: [], self: 'test', context, disabled: false, showIncomplete: false, onFocused() {}, onChange, onCreateKey() {}, onAddResource() {}, application: { id: 'test', name: 'Test', environments: bothConfigured(), workloads: { staging: [], production: [] } }, environment: 'staging' as const, onConfiguration() {}, onBlocked: blocked, onBusy() {}, onReplaceSecret() {} };
  const view = render(<EnvironmentVariablesEditor {...props} />); const user = userEvent.setup();
  await user.click(screen.getByRole('button', { name: 'Nhập từ file .env' }));
  fireEvent.change(screen.getByLabelText('Nội dung .env'), { target: { value: 'ADDED=@db.host' } });
  expect(onChange).not.toHaveBeenCalled();expect(blocked).toHaveBeenLastCalledWith(0, true);
  const ready: RowContext = { ...context, dependencies: [{ alias: 'db', type: 'postgres' }], outputs: () => ['host'] };
  view.rerender(<EnvironmentVariablesEditor {...props} context={ready} />);
  await waitFor(() => expect(onChange).toHaveBeenCalled());
  const next = onChange.mock.lastCall![0] as Binding[];
  expect(next.map(binding => ({ name: binding.name, source: binding.source, alias: binding.alias }))).toEqual([{ name: 'ADDED', source: 'RESOURCE', alias: 'db' }]);
  // Save stays locked until the parent has actually adopted these bindings.
  expect(blocked).toHaveBeenLastCalledWith(0, true);
  view.rerender(<EnvironmentVariablesEditor {...props} context={ready} bindings={next} />);
  await waitFor(() => expect(blocked).toHaveBeenLastCalledWith(0, false));
  await user.click(screen.getByRole('button', { name: 'Quay lại bảng' }));
  expect(screen.getByLabelText('Tên biến, dòng 1')).toHaveValue('ADDED');
});
