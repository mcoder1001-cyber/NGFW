import { fireEvent, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../test-utils.js';
import { SchemaForm } from './SchemaForm.js';
import type { JsonSchema } from './types.js';

const schema: JsonSchema = {
  type: 'object',
  properties: {
    mtu: { type: 'integer', title: 'MTU', minimum: 68 },
    lcp: {
      type: 'object',
      title: 'Automatic interface pair',
      'x-vrx-ui': { widget: 'hidden', group: 'Automatic routing' },
      properties: {
        hostIfName: { type: 'string', title: 'Host interface name' },
        hostIfType: { type: 'string', enum: ['tap', 'tun'], default: 'tap' },
      },
      required: ['hostIfName'],
    },
  },
};

describe('automatic hidden fields', () => {
  it('omits their controls and group while preserving their values on an adjacent field save', async () => {
    const onSubmit = vi.fn();
    const pair = { hostIfName: 'route0', hostIfType: 'tap' };
    renderWithProviders(
      <SchemaForm schema={schema} value={{ mtu: 1500, lcp: pair }} onSubmit={onSubmit} />,
    );
    expect(screen.queryByText('Automatic routing')).not.toBeInTheDocument();
    expect(screen.queryByText('Automatic interface pair')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Host interface name')).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('MTU'), { target: { value: '9000' } });
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith({ mtu: 9000, lcp: pair }));
  });

  it('keeps an absent optional hidden object absent when creating interface settings', async () => {
    const onSubmit = vi.fn();
    renderWithProviders(<SchemaForm schema={schema} value={{ mtu: 1500 }} onSubmit={onSubmit} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith({ mtu: 1500 }));
  });
});
