import { fireEvent, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { InterfaceLabelsProvider } from '../interface-labels.js';
import { renderWithProviders } from '../test-utils.js';
import { SchemaForm } from './SchemaForm.js';
import type { JsonSchema } from './types.js';

const labels = new Map([['wan', 'Ethernet1'], ['GigabitEthernet0/8/0', 'Ethernet1'], ['lan', 'Ethernet2'], ['ens192', 'Ethernet1']]);
const interfaceSchema: JsonSchema = { type: 'string', title: 'Port', 'x-ngfw-ui': { widget: 'interface-picker' } };
function renderForm(schema: JsonSchema, value: unknown, options = ['wan', 'lan']) {
  const onSubmit = vi.fn();
  renderWithProviders(<InterfaceLabelsProvider labels={labels}><SchemaForm schema={schema} value={value} interfaceOptions={options} onSubmit={onSubmit} /></InterfaceLabelsProvider>);
  return onSubmit;
}
describe('interface aliases in schema forms', () => {
  it('shows Ethernet names, searches raw names, and submits logical identities', async () => {
    const onSubmit = renderForm(interfaceSchema, 'wan');
    const input = screen.getByRole('combobox', { name: 'Port' });
    expect(input).toHaveValue('Ethernet1');
    await userEvent.clear(input);
    await userEvent.type(input, 'lan');
    await userEvent.click(await screen.findByRole('option', { name: 'Ethernet2' }));
    expect(input).toHaveValue('Ethernet2');
    fireEvent.submit(input.closest('form')!);
    await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith('lan'));
  });
  it('resolves a typed VLAN alias and preserves unknown free text', async () => {
    const onSubmit = renderForm(interfaceSchema, 'wan.100');
    const input = screen.getByRole('combobox', { name: 'Port' });
    expect(input).toHaveValue('Ethernet1.100');
    fireEvent.change(input, { target: { value: 'Ethernet2.200' } });
    fireEvent.blur(input);
    fireEvent.submit(input.closest('form')!);
    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith('lan.200'));
    fireEvent.change(input, { target: { value: 'future-port' } });
    fireEvent.blur(input);
    fireEvent.submit(input.closest('form')!);
    await waitFor(() => expect(onSubmit).toHaveBeenLastCalledWith('future-port'));
  });
  it('keeps host picker values as netdev names when the alias also identifies a dataplane port', async () => {
    const onSubmit = renderForm({ ...interfaceSchema, 'x-ngfw-ui': { widget: 'host-interface-picker' } }, 'ens192', ['ens192']);
    const input = screen.getByRole('combobox', { name: 'Port' });
    fireEvent.change(input, { target: { value: 'Ethernet1' } });
    fireEvent.blur(input);
    fireEvent.submit(input.closest('form')!);
    await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith('ens192'));
  });
  it('labels and selects interface chips without putting labels in payloads', async () => {
    const value = ['wan'];
    const onSubmit = renderForm({ type: 'array', title: 'Ports', items: interfaceSchema, 'x-ngfw-ui': { widget: 'chips' } }, value);
    expect(screen.getByText('Ethernet1')).toBeInTheDocument();
    const input = screen.getByRole('combobox', { name: 'Ports' });
    await userEvent.type(input, 'Ethernet2');
    await userEvent.click(await screen.findByRole('option', { name: 'Ethernet2' }));
    fireEvent.submit(input.closest('form')!);
    await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith(['wan', 'lan']));
    expect(value).toEqual(['wan']);
  });
  it('does not relabel unrelated record keys or descriptions that equal an interface name', async () => {
    const onSubmit = renderForm({ type: 'object', additionalProperties: { type: 'string' } }, { wan: 'lan' });
    expect(screen.getByRole('textbox', { name: /Key/ })).toHaveValue('wan');
    const value = screen.getByRole('textbox', { name: /Value/ });
    expect(value).toHaveValue('lan');
    fireEvent.submit(value.closest('form')!);
    await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith({ wan: 'lan' }));
  });
  it('labels record keys while retaining their submitted identities', async () => {
    const value = { wan: true };
    const onSubmit = renderForm({ type: 'object', propertyNames: interfaceSchema, additionalProperties: { type: 'boolean' } }, value);
    const input = screen.getByRole('combobox');
    expect(input).toHaveValue('Ethernet1');
    fireEvent.submit(input.closest('form')!);
    await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith({ wan: true }));
    expect(value).toEqual({ wan: true });
  });
});
