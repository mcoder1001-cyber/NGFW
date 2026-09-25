import { fireEvent, render, screen, waitFor, within, type RenderResult } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { describe, expect, it, vi } from 'vitest';
import { directionFor } from '../i18n/index.js';
import { createTestI18n, renderWithProviders } from '../test-utils.js';
import { VrxThemeProvider } from '../theme/VrxThemeProvider.js';
import { SchemaForm } from './SchemaForm.js';
import type { JsonSchema } from './types.js';

/** WEB-1: presence toggle, structured-string widgets, RTL-1, per-path i18n, itemKey summaries, rule-editor table. */

/** Like `renderWithProviders`, plus app namespaces (`bundles`) in the test i18n instance. */
function renderWithBundles(ui: ReactElement, lang: 'en' | 'fa', bundles: Record<string, Record<string, unknown>>): RenderResult {
  const i18n = createTestI18n(lang);
  for (const [ns, res] of Object.entries(bundles)) i18n.addResourceBundle(lang, ns, res, true, true);
  return render(ui, {
    wrapper: ({ children }) => (
      <I18nextProvider i18n={i18n}>
        <VrxThemeProvider mode="light" lang={lang} dir={directionFor(lang)}>
          {children}
        </VrxThemeProvider>
      </I18nextProvider>
    ),
  });
}

const save = () => fireEvent.click(screen.getByRole('button', { name: /^(Save|ذخیره)$/ }));

const PRESENCE: JsonSchema = {
  type: 'object',
  properties: {
    name: { type: 'string', title: 'Name' },
    dhcpClient: {
      type: 'object',
      title: 'DHCP client',
      properties: {
        hostname: { type: 'string', title: 'Hostname' },
        setBroadcastFlag: { type: 'boolean', title: 'Broadcast flag', default: false },
      },
      additionalProperties: false,
    },
  },
  required: ['name'],
  additionalProperties: false,
};

describe('<SchemaForm> presence of optional objects (P08-questions Q2)', () => {
  it('keeps an absent optional object absent until switched on, and drops it when switched off', { timeout: 90_000 }, async () => {
    const onSubmit = vi.fn();
    const { rerender } = renderWithProviders(<SchemaForm schema={PRESENCE} value={{ name: 'eth0' }} onSubmit={onSubmit} />);
    const presence = () => screen.getByLabelText('Configure DHCP client');
    expect(presence()).not.toBeChecked();
    expect(screen.getByText('Not configured')).toBeInTheDocument();
    expect(screen.queryByLabelText('Hostname')).toBeNull();
    save();
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    expect(onSubmit.mock.calls[0]![0]).toEqual({ name: 'eth0' }); // no phantom { setBroadcastFlag: false }

    fireEvent.click(presence());
    expect(presence()).toBeChecked();
    fireEvent.change(await screen.findByLabelText('Hostname'), { target: { value: 'h1' } });
    save();
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(2));
    expect(onSubmit.mock.calls[1]![0]).toEqual({ name: 'eth0', dhcpClient: { hostname: 'h1', setBroadcastFlag: false } });

    fireEvent.click(presence());
    expect(screen.queryByLabelText('Hostname')).toBeNull();
    save();
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(3));
    expect(onSubmit.mock.calls[2]![0]).toEqual({ name: 'eth0' });

    // a stored value opens switched on, with its defaults filled in
    rerender(<SchemaForm schema={PRESENCE} value={{ name: 'eth0', dhcpClient: { hostname: 'a' } }} onSubmit={onSubmit} />);
    await waitFor(() => expect(presence()).toBeChecked());
    expect(screen.getByLabelText('Hostname')).toHaveValue('a');
  });

  it('lists a server error for a field of an absent optional object instead of hiding it', { timeout: 60_000 }, async () => {
    renderWithProviders(
      <SchemaForm
        schema={PRESENCE}
        value={{ name: 'eth0' }}
        onSubmit={() => {}}
        problem={{ title: 'Validation failed', status: 422, errors: [{ pointer: '/dhcpClient/hostname', detail: 'hostname clash' }, { pointer: '/name', detail: 'name taken' }] }}
      />,
    );
    expect(await screen.findByText('name taken')).toBeInTheDocument(); // on its field
    expect(screen.getByRole('alert')).toHaveTextContent('At /dhcpClient/hostname: hostname clash');
  });
});

const WIDGETS: JsonSchema = {
  type: 'object',
  properties: {
    ports: { type: 'string', title: 'Ports', pattern: '^(\\d{1,5})(?:-(\\d{1,5}))?$', 'x-vrx-ui': { widget: 'port-range' } },
    pool: { type: 'string', title: 'Pool', pattern: '^(\\d{1,3}(?:\\.\\d{1,3}){3})(?:-(\\d{1,3}(?:\\.\\d{1,3}){3}))?$', 'x-vrx-ui': { widget: 'ip-range' } },
    at: { type: 'string', title: 'At', pattern: '^(?:[01]\\d|2[0-3]):[0-5]\\d$', 'x-vrx-ui': { widget: 'time' } },
    from: { type: 'string', title: 'Valid from', format: 'date-time', 'x-vrx-ui': { widget: 'datetime' } },
    tz: { type: 'string', title: 'Time zone', 'x-vrx-ui': { widget: 'timezone-picker' } },
    color: { type: 'string', title: 'Colour', pattern: '^#[0-9a-fA-F]{6}$', 'x-vrx-ui': { widget: 'color' } },
  },
  additionalProperties: false,
};

const WIDGET_VALUE = {
  ports: '443',
  pool: '10.0.0.10-10.0.0.20',
  at: '08:30',
  from: '2026-09-24T18:00:00+03:30',
  tz: 'Asia/Tehran',
  color: '#1e88e5',
};

describe('<SchemaForm> structured-string widgets', () => {
  it('edits port/ip ranges, time, date-time with offset, time zone and colour as the strings the schema validates', { timeout: 90_000 }, async () => {
    const onSubmit = vi.fn();
    renderWithProviders(<SchemaForm schema={WIDGETS} value={WIDGET_VALUE} onSubmit={onSubmit} />);
    const ports = screen.getByRole('group', { name: 'Ports' });
    expect(within(ports).getByLabelText('From')).toHaveValue('443');
    expect(within(ports).getByLabelText('To')).toHaveValue('');
    fireEvent.change(within(ports).getByLabelText('To'), { target: { value: '80a80' } }); // non-digits are dropped
    const pool = screen.getByRole('group', { name: 'Pool' });
    expect(within(pool).getByLabelText('From')).toHaveValue('10.0.0.10');
    expect(within(pool).getByLabelText('To')).toHaveValue('10.0.0.20');
    fireEvent.change(within(pool).getByLabelText('To'), { target: { value: '' } });

    expect(screen.getByLabelText('At')).toHaveValue('08:30');
    fireEvent.change(screen.getByLabelText('At'), { target: { value: '09:15' } });

    const from = screen.getByLabelText('Valid from');
    expect(from).toHaveValue('2026-09-24T18:00');
    expect(screen.getByLabelText('UTC offset')).toHaveValue('+03:30');
    fireEvent.change(from, { target: { value: '2026-10-01T07:05' } });

    expect(screen.getByLabelText('Time zone')).toHaveValue('Asia/Tehran');
    expect(screen.getByLabelText('Colour')).toHaveValue('#1e88e5');
    const swatch = screen.getByLabelText('Pick a colour');
    expect(swatch).toHaveValue('#1e88e5');
    fireEvent.change(swatch, { target: { value: '#ff0000' } });

    save();
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    expect(onSubmit.mock.calls[0]![0]).toEqual({
      ports: '443-8080',
      pool: '10.0.0.10',
      at: '09:15',
      from: '2026-10-01T07:05:00+03:30',
      tz: 'Asia/Tehran',
      color: '#ff0000',
    });
  });

  it('keeps a partial or cleared UTC offset in its input while typing; validates it on submit (review M1)', { timeout: 90_000 }, async () => {
    const onSubmit = vi.fn();
    renderWithProviders(<SchemaForm schema={WIDGETS} value={WIDGET_VALUE} onSubmit={onSubmit} />);
    const offset = () => screen.getByLabelText('UTC offset');
    const from = () => screen.getByLabelText('Valid from');
    await userEvent.clear(offset());
    expect(offset()).toHaveValue(''); // still the structured inputs, not the raw-text fallback
    expect(from()).toHaveAttribute('type', 'datetime-local');
    await userEvent.type(offset(), '+04:');
    expect(offset()).toHaveValue('+04:');
    expect(offset()).toHaveFocus();
    expect(from()).toHaveValue('2026-09-24T18:00');
    save();
    expect(await screen.findByText('Does not match the required format')).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
    await userEvent.type(offset(), '30');
    expect(offset()).toHaveValue('+04:30');
    save();
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    expect(onSubmit.mock.calls[0]![0]).toMatchObject({ from: '2026-09-24T18:00:00+04:30' });
  });

  it('pasting a whole range into either end fills both ends (review L2)', { timeout: 60_000 }, async () => {
    const onSubmit = vi.fn();
    renderWithProviders(<SchemaForm schema={WIDGETS} value={{}} onSubmit={onSubmit} />);
    const ports = screen.getByRole('group', { name: 'Ports' });
    fireEvent.change(within(ports).getByLabelText('From'), { target: { value: '8000-8080' } });
    expect(within(ports).getByLabelText('From')).toHaveValue('8000');
    expect(within(ports).getByLabelText('To')).toHaveValue('8080');
    const pool = screen.getByRole('group', { name: 'Pool' });
    fireEvent.change(within(pool).getByLabelText('To'), { target: { value: '10.0.0.10-10.0.0.20' } });
    expect(within(pool).getByLabelText('From')).toHaveValue('10.0.0.10');
    expect(within(pool).getByLabelText('To')).toHaveValue('10.0.0.20');
    save();
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    expect(onSubmit.mock.calls[0]![0]).toEqual({ ports: '8000-8080', pool: '10.0.0.10-10.0.0.20' });
  });

  it('typing a dash in one end never clears the other end (verify F1)', { timeout: 60_000 }, async () => {
    renderWithProviders(<SchemaForm schema={WIDGETS} value={{ ports: '8000-8080' }} onSubmit={() => {}} />);
    const ports = screen.getByRole('group', { name: 'Ports' });
    const from = within(ports).getByLabelText('From');
    const to = within(ports).getByLabelText('To');
    await userEvent.type(from, '-');
    expect(from).toHaveValue('8000');
    expect(to).toHaveValue('8080');
    await userEvent.type(to, '-');
    expect(from).toHaveValue('8000');
    expect(to).toHaveValue('8080');
  });

  it('suggests IANA time zones with UTC first', { timeout: 60_000 }, async () => {
    renderWithProviders(<SchemaForm schema={WIDGETS} value={{}} onSubmit={() => {}} />);
    const tz = screen.getByLabelText('Time zone');
    await userEvent.type(tz, 'Teh');
    expect(await screen.findByRole('option', { name: 'Asia/Tehran' })).toBeInTheDocument();
    await userEvent.clear(tz);
    await userEvent.type(tz, 'UT');
    expect(await screen.findByRole('option', { name: 'UTC' })).toBeInTheDocument();
  });
});

const RTL_SCHEMA: JsonSchema = {
  type: 'object',
  properties: {
    description: { type: 'string', title: 'Description', 'x-vrx-ui': { widget: 'textarea' } },
    label: { type: 'string', title: 'Label', maxLength: 63 },
    vrf: { type: 'string', title: 'VRF', 'x-vrx-ui': { widget: 'vrf-picker' } },
    object: { type: 'string', title: 'Object', pattern: '^[A-Za-z0-9][A-Za-z0-9_.-]*$' },
    host: { type: 'string', title: 'Host', format: 'hostname' },
    ports: { type: 'string', title: 'Ports', 'x-vrx-ui': { widget: 'port-range', help: '443 or 8000-8080' } },
    at: { type: 'string', title: 'At', 'x-vrx-ui': { widget: 'time' } },
    prefixes: { type: 'array', title: 'Prefixes', items: { type: 'string', format: 'cidrv6', 'x-vrx-ui': { widget: 'cidr' } }, 'x-vrx-ui': { widget: 'chips' } },
    tags: { type: 'array', title: 'Tags', items: { type: 'string' }, 'x-vrx-ui': { widget: 'tag-picker' } },
  },
};

describe('<SchemaForm> RTL-1', () => {
  it('renders identifier values LTR inside an RTL page; prose follows the page', { timeout: 60_000 }, () => {
    renderWithProviders(
      <SchemaForm schema={RTL_SCHEMA} value={{ prefixes: ['2001:db8::/64'], tags: ['web'], ports: '80' }} onSubmit={() => {}} />,
      { lang: 'fa' },
    );
    expect(document.documentElement).toHaveAttribute('dir', 'rtl');
    for (const label of ['VRF', 'Object', 'Host', 'At']) {
      expect(screen.getByLabelText(label), label).toHaveAttribute('dir', 'ltr');
    }
    const ports = screen.getByRole('group', { name: 'Ports' });
    for (const input of within(ports).getAllByRole('textbox')) expect(input).toHaveAttribute('dir', 'ltr');
    expect(screen.getByLabelText('Description', { exact: false })).not.toHaveAttribute('dir');
    expect(screen.getByLabelText('Label', { exact: false })).not.toHaveAttribute('dir');
    // identifier chips are isolated LTR so `2001:db8::/64` is never reordered
    expect(screen.getByText('2001:db8::/64').closest('bdi')).toHaveAttribute('dir', 'ltr');
    expect(screen.getByText('web').closest('bdi')).toHaveAttribute('dir', 'ltr');
    expect(screen.getByLabelText('Prefixes', { exact: false })).toHaveAttribute('dir', 'ltr');
    // help is prose in the UI language: its direction is the locale's, set explicitly (review M2)
    const help = screen.getByText('443 or 8000-8080');
    expect(help.tagName).toBe('SPAN');
    expect(help).toHaveAttribute('dir', 'rtl');
  });

  it('Persian help that starts with a Latin word stays RTL (P08 fa MTU / RX mode help, review M2)', { timeout: 60_000 }, () => {
    const schema: JsonSchema = {
      type: 'object',
      properties: {
        mtu: { type: 'integer', title: 'MTU', 'x-vrx-ui': { help: 'MTU لایه‌ی ۳ به بایت (۶۸ تا ۹۲۱۶)؛ خالی = مقدار پیش‌فرض درایور' } },
        rxMode: { type: 'string', title: 'RX mode', enum: ['polling', 'interrupt'], 'x-vrx-ui': { help: 'polling (پیش‌فرض DPDK)، interrupt یا adaptive' } },
      },
    };
    renderWithProviders(<SchemaForm schema={schema} value={{}} onSubmit={() => {}} />, { lang: 'fa' });
    for (const text of ['MTU لایه‌ی ۳ به بایت (۶۸ تا ۹۲۱۶)؛ خالی = مقدار پیش‌فرض درایور', 'polling (پیش‌فرض DPDK)، interrupt یا adaptive']) {
      const el = screen.getByText(text);
      expect(el, text).toHaveAttribute('dir', 'rtl'); // not `<bdi>` guessing LTR from the first strong character
      expect(el.closest('bdi'), text).toBeNull();
    }
  });

  it('the same help in an English page is LTR', { timeout: 60_000 }, () => {
    renderWithProviders(<SchemaForm schema={RTL_SCHEMA} value={{}} onSubmit={() => {}} />);
    expect(screen.getByText('443 or 8000-8080')).toHaveAttribute('dir', 'ltr');
  });
});

const I18N_SCHEMA: JsonSchema = {
  type: 'object',
  properties: {
    role: { type: 'string', title: 'Role', enum: ['admin', 'operator'], 'x-vrx-ui': { group: 'Security' } },
    scope: { type: 'string', title: 'Scope', 'x-vrx-ui': { group: 'Security', help: 'Untranslated help' } },
    auth: {
      title: 'Authentication',
      oneOf: [
        { type: 'object', title: 'PSK', properties: { kind: { const: 'psk' } }, required: ['kind'] },
        { type: 'object', title: 'Certificate', properties: { kind: { const: 'cert' } }, required: ['kind'] },
      ],
    },
    rules: {
      type: 'array',
      title: 'Rules',
      'x-vrx-ui': { widget: 'rule-editor' },
      items: {
        type: 'object',
        properties: { action: { type: 'string', title: 'Action', enum: ['permit', 'deny'] } },
        required: ['action'],
      },
    },
  },
};

const DEMO_FA = {
  field: {
    role: { title: 'نقش', help: 'سطح دسترسی', enum: { admin: 'مدیر' } },
    auth: { title: 'احراز هویت', variant: { psk: 'کلید مشترک' } },
    group: { Security: 'امنیت' },
    rules: { title: 'قواعد', action: { title: 'اقدام', enum: { permit: 'اجازه' } } },
  },
};

describe('<SchemaForm i18nPrefix> per-path texts (I18N-1)', () => {
  it('translates titles, help, enum and variant labels, group legends and table headers; falls back to the schema', { timeout: 90_000 }, async () => {
    renderWithBundles(
      <SchemaForm schema={I18N_SCHEMA} value={{ role: 'admin', auth: { kind: 'psk' }, rules: [{ action: 'permit' }, { action: 'deny' }] }} onSubmit={() => {}} i18nPrefix="demo:field" />,
      'fa',
      { demo: DEMO_FA },
    );
    expect(screen.getByRole('group', { name: 'امنیت' })).toBeInTheDocument();
    const role = screen.getByLabelText('نقش', { exact: false });
    expect(role).toHaveTextContent('مدیر');
    expect(screen.getByText('سطح دسترسی')).toBeInTheDocument();
    expect(screen.getByLabelText('Scope', { exact: false })).toBeInTheDocument(); // no key → schema title
    expect(screen.getByText('Untranslated help')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'احراز هویت' })).toBeInTheDocument();
    const table = screen.getByRole('table', { name: 'قواعد' });
    expect(within(table).getByRole('columnheader', { name: 'اقدام' })).toBeInTheDocument();
    const [, first, second] = within(table).getAllByRole('row');
    expect(first).toHaveTextContent('اجازه');
    expect(second).toHaveTextContent('deny'); // no label → the raw value
    await userEvent.click(role);
    expect(await screen.findByRole('option', { name: 'مدیر' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'operator' })).toBeInTheDocument();
    await userEvent.keyboard('{Escape}');
    await userEvent.click(screen.getByLabelText('نوع', { exact: false }));
    expect(await screen.findByRole('option', { name: 'کلید مشترک' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Certificate' })).toBeInTheDocument();
  });
});

const USERS: JsonSchema = {
  type: 'object',
  properties: {
    users: {
      type: 'array',
      title: 'Users',
      'x-vrx-ui': { itemKey: ['username'] },
      items: {
        type: 'object',
        title: 'User',
        properties: {
          username: { type: 'string', title: 'Username', minLength: 1 },
          role: { type: 'string', title: 'Role', enum: ['admin', 'operator'], default: 'operator' },
        },
        required: ['username', 'role'],
      },
    },
  },
};

describe('<SchemaForm> itemKey row summaries', () => {
  it('shows each row by its key, collapsed; opens on demand; a new row stays open while its key is typed', { timeout: 90_000 }, async () => {
    const onSubmit = vi.fn();
    renderWithProviders(
      <SchemaForm schema={USERS} value={{ users: [{ username: 'alice', role: 'admin' }, { username: 'bob', role: 'operator' }] }} onSubmit={onSubmit} />,
    );
    expect(screen.getByRole('heading', { name: 'User alice' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'User bob' })).toBeInTheDocument();
    expect(screen.queryAllByLabelText('Username', { exact: false })).toHaveLength(0);
    fireEvent.click(screen.getByRole('button', { name: 'Open row 1' }));
    expect(screen.getByLabelText('Username', { exact: false })).toHaveValue('alice');
    fireEvent.click(screen.getByRole('button', { name: 'Close row 1' }));
    expect(screen.queryAllByLabelText('Username', { exact: false })).toHaveLength(0);

    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    const fresh = await screen.findByLabelText('Username', { exact: false });
    fireEvent.change(fresh, { target: { value: 'c' } });
    expect(screen.getByLabelText('Username', { exact: false })).toHaveValue('c'); // still open with a summary now
    expect(screen.getByRole('heading', { name: 'User c' })).toBeInTheDocument();
    save();
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    expect(onSubmit.mock.calls[0]![0]).toEqual({
      users: [
        { username: 'alice', role: 'admin' },
        { username: 'bob', role: 'operator' },
        { username: 'c', role: 'operator' },
      ],
    });
  });
});

const ACL: JsonSchema = {
  type: 'object',
  properties: {
    rules: {
      type: 'array',
      title: 'Rules',
      'x-vrx-ui': { widget: 'rule-editor' },
      items: {
        type: 'object',
        title: 'ACL rule',
        properties: {
          sequence: { type: 'integer', title: 'Sequence', minimum: 1 },
          description: { type: 'string', title: 'Description', 'x-vrx-ui': { widget: 'textarea' } },
          action: { type: 'string', title: 'Action', enum: ['permit', 'deny'] },
          source: {
            title: 'Source',
            default: { kind: 'any' },
            oneOf: [
              { type: 'object', properties: { kind: { const: 'any' } }, required: ['kind'], additionalProperties: false },
              {
                type: 'object',
                properties: { kind: { const: 'prefix' }, prefix: { type: 'string', title: 'Prefix' } },
                required: ['kind', 'prefix'],
                additionalProperties: false,
              },
            ],
          },
          log: { type: 'boolean', title: 'Log', default: false },
        },
        required: ['sequence', 'action'],
        additionalProperties: false,
      },
    },
  },
};

describe('<SchemaForm> rule-editor table', () => {
  it('lists rules as table rows, edits one beneath its row, reorders, marks rows with errors', { timeout: 120_000 }, async () => {
    const onSubmit = vi.fn();
    renderWithProviders(
      <SchemaForm
        schema={ACL}
        value={{
          rules: [
            { sequence: 10, action: 'permit', source: { kind: 'any' }, description: 'web' },
            { sequence: 20, action: 'deny', source: { kind: 'prefix', prefix: '10.0.0.0/8' } },
          ],
        }}
        onSubmit={onSubmit}
      />,
    );
    const table = screen.getByRole('table', { name: 'Rules' });
    const headers = within(table).getAllByRole('columnheader').map((h) => h.textContent);
    expect(headers).toEqual(['#', 'Sequence', 'Action', 'Source', 'Log', 'Actions']); // textarea stays in the row form
    const rows = () => within(table).getAllByRole('row').slice(1);
    expect(rows()[0]).toHaveTextContent(/^110permitanyNo/);
    expect(rows()[1]).toHaveTextContent(/^220denyprefix 10\.0\.0\.0\/8No/);
    expect(screen.queryByLabelText('Sequence', { exact: false })).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'Edit row 2' }));
    const seq = screen.getByLabelText('Sequence', { exact: false });
    expect(seq).toHaveValue(20);
    fireEvent.change(seq, { target: { value: '5' } });
    expect(rows()[1]).toHaveTextContent(/^25deny/);
    fireEvent.click(within(rows()[0]!).getByRole('button', { name: 'Move down' }));
    expect(rows()[0]).toHaveTextContent(/^15deny/);

    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    await waitFor(() => expect(within(table).getAllByRole('row').length).toBeGreaterThan(4));
    save();
    await waitFor(() => expect(screen.getAllByTitle('This row has errors').length).toBeGreaterThan(0));
    expect(onSubmit).not.toHaveBeenCalled();
    const newRow = rows().find((r) => r.textContent?.startsWith('3'))!;
    fireEvent.click(within(newRow).getByRole('button', { name: 'Remove' }));
    save();
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    expect(onSubmit.mock.calls[0]![0]).toEqual({
      rules: [
        { sequence: 5, action: 'deny', source: { kind: 'prefix', prefix: '10.0.0.0/8' }, log: false },
        { sequence: 10, action: 'permit', source: { kind: 'any' }, description: 'web', log: false },
      ],
    });
  });
});

describe('<SchemaForm> cleared inputs', () => {
  it('a cleared pre-filled input stays empty (no snap-back to the stored value) and is submitted as absent', { timeout: 60_000 }, async () => {
    const onSubmit = vi.fn();
    const schema: JsonSchema = { type: 'object', properties: { name: { type: 'string', title: 'Name' }, mtu: { type: 'integer', title: 'MTU' } } };
    renderWithProviders(<SchemaForm schema={schema} value={{ name: 'eth0', mtu: 1500 }} onSubmit={onSubmit} />);
    const name = screen.getByLabelText('Name');
    await userEvent.clear(name);
    expect(name).toHaveValue('');
    await userEvent.type(name, 'wan');
    expect(name).toHaveValue('wan'); // replaced, not appended to "eth0"
    const mtu = screen.getByLabelText('MTU');
    fireEvent.change(mtu, { target: { value: '' } }); // "empty = keep the driver default"
    expect(mtu).toHaveValue(null);
    save();
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    expect(onSubmit.mock.calls[0]![0]).toEqual({ name: 'wan' });
  });
});

describe('<SchemaForm> rows opened by the form stay open while the user types (review M3)', () => {
  it('rule-editor: a row opened by a validation error stays open, with the focus, once the value is fixed', { timeout: 90_000 }, async () => {
    const onSubmit = vi.fn();
    renderWithProviders(<SchemaForm schema={ACL} value={{ rules: [{ sequence: 0, action: 'permit' }] }} onSubmit={onSubmit} />);
    expect(screen.queryByLabelText('Sequence', { exact: false })).toBeNull(); // starts closed
    save();
    const seq = await screen.findByLabelText('Sequence', { exact: false }); // opened by the error
    await userEvent.clear(seq);
    await userEvent.type(seq, '7');
    await waitFor(() => expect(screen.queryAllByTitle('This row has errors')).toHaveLength(0));
    expect(screen.getByLabelText('Sequence', { exact: false })).toBe(seq); // still open, same input
    expect(seq).toHaveFocus();
    expect(screen.getByRole('button', { name: 'Close row 1' })).toBeEnabled(); // now the user may close it
    save();
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    expect(onSubmit.mock.calls[0]![0]).toMatchObject({ rules: [{ sequence: 7, action: 'permit' }] });
  });

  it('itemKey list: a stored row without a key stays open while its first characters are typed', { timeout: 90_000 }, async () => {
    renderWithProviders(<SchemaForm schema={USERS} value={{ users: [{ username: 'alice', role: 'admin' }, { username: '', role: 'operator' }] }} onSubmit={() => {}} />);
    const inputs = screen.getAllByLabelText('Username', { exact: false }); // only the keyless row is open
    expect(inputs).toHaveLength(1);
    const input = inputs[0]!;
    await userEvent.type(input, 'bo');
    expect(screen.getByRole('heading', { name: 'User bo' })).toBeInTheDocument();
    expect(screen.getByLabelText('Username', { exact: false })).toBe(input);
    expect(input).toHaveFocus();
    expect(input).toHaveValue('bo');
  });

  it('a row opened by a server error pointer stays open after the user fixes the field and leaves it', { timeout: 90_000 }, async () => {
    renderWithProviders(
      <SchemaForm
        schema={USERS}
        value={{ users: [{ username: 'alice', role: 'admin' }] }}
        onSubmit={() => {}}
        problem={{ title: 'Validation failed', status: 422, errors: [{ pointer: '/users/0/username', detail: 'username taken' }] }}
      />,
    );
    expect(await screen.findByText('username taken')).toBeInTheDocument();
    const input = screen.getByLabelText('Username', { exact: false });
    await userEvent.clear(input);
    await userEvent.type(input, 'alice2');
    await userEvent.tab();
    await waitFor(() => expect(screen.queryByText('username taken')).toBeNull());
    expect(screen.getByLabelText('Username', { exact: false })).toHaveValue('alice2');
  });
});
