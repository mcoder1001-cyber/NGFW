import { fireEvent, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../test-utils.js';
import { SchemaForm } from './SchemaForm.js';
import type { JsonSchema } from './types.js';

describe('direct generic engine editor', () => {
  it.each(['en', 'fa'] as const)(
    'labels root enum choices in %s and submits the unchanged API value',
    async (lang) => {
      const onSubmit = vi.fn();
      const schema: JsonSchema = {
        type: 'string',
        title: 'IKE engine',
        enum: ['strongswan', 'vpp-ikev2'],
      };
      renderWithProviders(<SchemaForm schema={schema} value="strongswan" onSubmit={onSubmit} />, {
        lang,
      });
      const picker = screen.getByRole('combobox');
      expect(picker).toHaveTextContent(lang === 'fa' ? 'سرویس IPsec' : 'IPsec service');
      await userEvent.click(picker);
      const choice = screen.getByRole('option', {
        name: lang === 'fa' ? 'IKEv2 داخلی' : 'Native IKEv2',
      });
      expect(document.body.textContent).not.toMatch(/strongswan|vpp/i);
      await userEvent.click(choice);
      fireEvent.submit(picker.closest('form')!);
      await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith('vpp-ikev2'));
      expect(schema.enum).toEqual(['strongswan', 'vpp-ikev2']);
    },
  );

  it('preserves explicit labels for unrelated root enum choices', async () => {
    const onSubmit = vi.fn();
    renderWithProviders(
      <SchemaForm
        schema={{
          type: 'string',
          title: 'Mode',
          enum: ['stable', 'test'],
          'x-ngfw-ui': { enumLabels: { stable: 'Production', test: 'Testing' } },
        }}
        value="stable"
        onSubmit={onSubmit}
      />,
    );
    const picker = screen.getByRole('combobox');
    expect(picker).toHaveTextContent('Production');
    fireEvent.submit(picker.closest('form')!);
    await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith('stable'));
  });
});
