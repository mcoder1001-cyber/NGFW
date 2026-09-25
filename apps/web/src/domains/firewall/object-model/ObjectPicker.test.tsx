import { VrxThemeProvider } from '@ngfw/ui-kit';
import { SchemaForm, type JsonSchema } from '@ngfw/ui-kit/schema-form';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, it } from 'vitest';
import i18n from '../../../i18n';
import { domainSchemas } from '../../../schema/registry';
import { installFakeApi, resetSession } from '../../../test-api';
import { pickerKinds } from './model';
import { objectModelWidgets } from './ObjectPicker';

/**
 * Review F4: the exported picker never guesses. F-acl's attachment form (the real `acl.attachments[]` item schema) marks
 * `list` (an ACL list name) and `target.zone` (a zone) as `object-picker`: the zone becomes a select of the zones, the
 * list stays the plain text field — no address objects offered.
 */
const attachmentSchema = (domainSchemas.acl.properties?.['attachments'] as { items: JsonSchema }).items;

afterEach(async () => {
  await resetSession();
});

describe('ObjectPicker in F-acl attachment forms', () => {
  it('classifies target.zone as zones and leaves attachments[].list unclassified', () => {
    const props = attachmentSchema.properties ?? {};
    expect(props['list']?.['x-vrx-ui']).toMatchObject({ widget: 'object-picker' });
    expect(pickerKinds(props['list']?.['x-vrx-ui'] ?? {}, 'list')).toEqual([]);
    expect(pickerKinds({ widget: 'object-picker' }, 'target.zone')).toEqual(['zones']);
    for (const kind of ['macipAttachments', 'hostAttachments'] as const) {
      const item = (domainSchemas.acl.properties?.[kind] as { items: JsonSchema }).items;
      expect(pickerKinds(item.properties?.['list']?.['x-vrx-ui'] ?? {}, 'list'), kind).toEqual([]);
    }
  });

  it('renders the list as a plain text field and the zone as a select of zones', { timeout: 30_000 }, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/config/candidate/objects', {
      body: {
        addresses: { web1: { type: 'host', address: '192.0.2.10', tags: [] } },
        zones: { lan: { interfaces: ['host-w3l0'], tags: [] } },
      },
    });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={qc}>
          <VrxThemeProvider mode="light" lang="en" dir="ltr">
            <SchemaForm
              schema={attachmentSchema}
              widgets={objectModelWidgets}
              value={{ list: 'web-in', target: { kind: 'zone', zone: 'lan' }, direction: 'in', sequence: 1, enabled: true }}
              onSubmit={() => undefined}
            />
          </VrxThemeProvider>
        </QueryClientProvider>
      </I18nextProvider>,
    );
    const list = await screen.findByRole('textbox', { name: /^Access list/ });
    expect((list as HTMLInputElement).value).toBe('web-in');
    const zone = await screen.findByRole('combobox', { name: /^Zone/ });
    expect(zone).toHaveTextContent('lan');
    expect(within(document.body).queryByText(/web1/)).toBeNull(); // no address object anywhere in the form
  });
});
