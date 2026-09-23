import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../test-utils.js';
import { SchemaForm } from './SchemaForm.js';
import { WIDGET_SCHEMA, WIDGET_VALUE } from './test-schema.js';

describe('<SchemaForm>', () => {
  it('renders a control for every widget', () => {
    renderWithProviders(<SchemaForm schema={WIDGET_SCHEMA} value={WIDGET_VALUE} onSubmit={() => {}} interfaceOptions={['loop0']} />);
    expect(screen.getAllByLabelText(/^Name/)[0]).toHaveValue('eth0');
    for (const label of ['Description', 'Secret', 'Enabled', 'Monitored', 'MTU', 'RX mode', 'Gateway', 'MAC', 'Parent interface', 'Tags', 'Features', 'Extra', 'VRF']) {
      expect(screen.getByLabelText(label, { exact: false })).toBeInTheDocument();
    }
    expect(screen.getByRole('slider')).toBeInTheDocument(); // weight
    expect(screen.getByRole('radiogroup')).toBeInTheDocument(); // duplex
    expect(screen.getByRole('group', { name: 'IPv4 addresses' })).toBeInTheDocument(); // primitive list
    expect(screen.getByRole('group', { name: 'DNS servers' })).toBeInTheDocument(); // object list
    expect(screen.getByRole('group', { name: 'Security' })).toBeInTheDocument(); // x-vrx-ui.group
    expect(screen.getByDisplayValue('Gig0/0/0.100')).toBeInTheDocument(); // record key
    expect(screen.getByLabelText('VLAN', { exact: false })).toHaveValue(100);
    expect(screen.getByLabelText('Type', { exact: false })).toHaveTextContent('Pre-shared key'); // oneOf picker
    expect(screen.getByLabelText('PSK', { exact: false })).toHaveAttribute('type', 'password');
    expect(screen.queryByLabelText('Internal')).toBeNull(); // hidden
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled();
  });

  it('submits the parsed JSON: defaults applied, records back to objects', async () => {
    const onSubmit = vi.fn();
    renderWithProviders(<SchemaForm schema={WIDGET_SCHEMA} value={WIDGET_VALUE} onSubmit={onSubmit} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    expect(onSubmit.mock.calls[0]![0]).toEqual({ ...WIDGET_VALUE, enabled: true, weight: 5, rxMode: 'adaptive' });
  });

  it('shows translated validation messages on the offending field and blocks submit', async () => {
    const onSubmit = vi.fn();
    renderWithProviders(<SchemaForm schema={WIDGET_SCHEMA} value={WIDGET_VALUE} onSubmit={onSubmit} />);
    const mtu = screen.getByLabelText('MTU', { exact: false });
    fireEvent.change(mtu, { target: { value: '20' } }); // userEvent.clear() cannot clear type=number in jsdom
    fireEvent.blur(mtu);
    expect(await screen.findByText('Must be at least 68')).toBeInTheDocument();
    await userEvent.clear(screen.getAllByLabelText(/^Name/)[0]!);
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('Required')).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('maps RFC 9457 pointers onto fields (escaped record keys) and lists the rest', async () => {
    renderWithProviders(
      <SchemaForm
        schema={WIDGET_SCHEMA}
        value={WIDGET_VALUE}
        onSubmit={() => {}}
        problem={{
          type: 'about:blank',
          title: 'Validation failed',
          status: 422,
          detail: 'Two problems',
          errors: [
            { pointer: '/subs/Gig0~10~10.100/vlanId', detail: 'VLAN 100 is already used' },
            { pointer: '/does/not/exist', detail: 'unmappable' },
          ],
        }}
      />,
    );
    expect(await screen.findByText('VLAN 100 is already used')).toBeInTheDocument();
    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent('The server rejected the change');
    expect(alert).toHaveTextContent('At /does/not/exist: unmappable');
  });

  it('honours dependsOn: the VRF field follows the Enabled switch', async () => {
    renderWithProviders(<SchemaForm schema={WIDGET_SCHEMA} value={WIDGET_VALUE} onSubmit={() => {}} />);
    expect(screen.getByLabelText('VRF', { exact: false })).toBeInTheDocument();
    await userEvent.click(screen.getByLabelText('Enabled'));
    expect(screen.queryByLabelText('VRF', { exact: false })).toBeNull();
    await userEvent.click(screen.getByLabelText('Enabled'));
    expect(screen.getByLabelText('VRF', { exact: false })).toBeInTheDocument();
  });

  it('switches oneOf variants and submits the new shape', async () => {
    const onSubmit = vi.fn();
    renderWithProviders(<SchemaForm schema={WIDGET_SCHEMA} value={WIDGET_VALUE} onSubmit={onSubmit} />);
    await userEvent.click(screen.getByLabelText('Type', { exact: false }));
    await userEvent.click(await screen.findByRole('option', { name: 'Certificate' }));
    const cert = await screen.findByLabelText('Certificate name', { exact: false });
    await userEvent.type(cert, 'vrx-a');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(onSubmit).toHaveBeenCalled());
    expect(onSubmit.mock.calls[0]![0]).toMatchObject({ auth: { kind: 'cert', cert: 'vrx-a' } });
  });

  it('adds and removes record entries and list items', async () => {
    renderWithProviders(<SchemaForm schema={WIDGET_SCHEMA} value={WIDGET_VALUE} onSubmit={() => {}} />);
    const subs = screen.getByRole('heading', { name: 'Sub-interfaces' }).parentElement!;
    await userEvent.click(within(subs).getByRole('button', { name: 'Add' }));
    expect(within(subs).getAllByLabelText('VLAN', { exact: false })).toHaveLength(2);
    await userEvent.click(within(subs).getAllByRole('button', { name: 'Remove' })[1]!);
    expect(within(subs).getAllByLabelText('VLAN', { exact: false })).toHaveLength(1);
    const ipv4 = screen.getByRole('group', { name: 'IPv4 addresses' });
    await userEvent.click(within(ipv4).getByRole('button', { name: 'Add' }));
    await userEvent.type(within(ipv4).getByLabelText('Item 1', { exact: false }), '10.0.0.1/33');
    await userEvent.tab();
    expect(await screen.findByText('Enter an IPv4 prefix, e.g. 10.0.0.1/24')).toBeInTheDocument();
  });

  it('renders Persian labels right-to-left', () => {
    renderWithProviders(<SchemaForm schema={WIDGET_SCHEMA} value={WIDGET_VALUE} onSubmit={() => {}} />, { lang: 'fa' });
    expect(screen.getByRole('button', { name: 'ذخیره' })).toBeInTheDocument();
    expect(document.documentElement).toHaveAttribute('dir', 'rtl');
    expect(document.documentElement).toHaveAttribute('lang', 'fa');
  });

  it('read-only mode disables saving', () => {
    renderWithProviders(<SchemaForm schema={WIDGET_SCHEMA} value={WIDGET_VALUE} onSubmit={() => {}} readOnly />);
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
    expect(screen.getAllByLabelText(/^Name/)[0]).toHaveAttribute('readonly');
  });
});
