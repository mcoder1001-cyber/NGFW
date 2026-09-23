import { fireEvent, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { it } from 'vitest';
import { renderWithProviders } from '../test-utils.js';
import { SchemaForm } from './SchemaForm.js';
import { WIDGET_SCHEMA, WIDGET_VALUE } from './test-schema.js';

it('debug list blur validation', async () => {
  renderWithProviders(<SchemaForm schema={WIDGET_SCHEMA} value={WIDGET_VALUE} onSubmit={() => {}} />);
  const mtu = screen.getByLabelText('MTU', { exact: false }) as HTMLInputElement;
  fireEvent.change(mtu, { target: { value: '20' } });
  console.log('mtu now', mtu.value);
  const ipv4 = screen.getByRole('group', { name: 'IPv4 addresses' });
  await userEvent.click(within(ipv4).getByRole('button', { name: 'Add' }));
  const item = within(ipv4).getByLabelText('Item 1', { exact: false }) as HTMLInputElement;
  console.log('item type', item.type, 'value', JSON.stringify(item.value));
  await userEvent.type(item, '10.0.0.1/33');
  console.log('item after type', JSON.stringify(item.value), document.activeElement === item);
  await userEvent.tab();
  console.log('active after tab', document.activeElement?.tagName, document.activeElement?.getAttribute('aria-label'));
  await new Promise((r) => setTimeout(r, 2500));
  console.log('group text:', ipv4.textContent);
  console.log('body Must:', document.body.textContent?.includes('Must'));
});
