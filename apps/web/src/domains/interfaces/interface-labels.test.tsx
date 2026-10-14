import { InterfaceLabelsProvider, NgfwThemeProvider } from '@ngfw/ui-kit';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import '../../i18n';
import { RecordDialog } from './bridge-l2/RecordDialog';
import type { SubinterfaceConfig } from './model';
import { SubinterfaceTable } from './subinterfaces/SubinterfaceTable';

const PORT = 'GigabitEthernet0/8/0';
const LABELS = new Map([[PORT, 'Ethernet4']]);

function labeled(children: ReactNode) {
  return (
    <NgfwThemeProvider mode="light" lang="en" dir="ltr">
      <InterfaceLabelsProvider labels={LABELS}>{children}</InterfaceLabelsProvider>
    </NgfwThemeProvider>
  );
}

describe('interface labels in consumer controls', () => {
  it('shows a parent alias for pending subinterfaces and keeps the subinterface ID in actions', () => {
    const onEdit = vi.fn();
    const onRemove = vi.fn();
    const sub: SubinterfaceConfig = {
      enabled: true,
      vlanId: 100,
      dot1ad: false,
      ipv4: [],
      ipv6: [],
      vrf: 'default',
    };
    render(
      labeled(
        <SubinterfaceTable
          parent={PORT}
          subs={[['100', sub]]}
          items={[]}
          readOnly={false}
          canAdd
          busy={false}
          error={null}
          onAdd={vi.fn()}
          onEdit={onEdit}
          onRemove={onRemove}
        />,
      ),
    );
    const table = screen.getByRole('table', { name: 'Sub-interfaces of Ethernet4' });
    fireEvent.click(within(table).getByText('Ethernet4.100'));
    expect(onEdit).toHaveBeenCalledWith('100', sub);
    fireEvent.click(within(table).getByRole('button', { name: 'Remove sub-interface 100' }));
    expect(onRemove).toHaveBeenCalledWith('100');
    expect(screen.queryByText(PORT)).not.toBeInTheDocument();
  });

  it('labels an interface picker with Ethernet and submits the original interface key', async () => {
    const onSubmit = vi.fn();
    render(
      labeled(
        <RecordDialog
          open
          title="Add member"
          keyLabel="Interface"
          keyOptions={[PORT]}
          schema={{ type: 'object', properties: {} }}
          value={{}}
          error={null}
          pending={false}
          readOnly={false}
          pointerOf={(key) => `/interfaces/${key}`}
          onClose={vi.fn()}
          onSubmit={onSubmit}
        />,
      ),
    );
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Interface' }));
    fireEvent.click(within(screen.getByRole('listbox')).getByRole('option', { name: 'Ethernet4' }));
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith(PORT, {}));
    expect(screen.queryByText(PORT)).not.toBeInTheDocument();
  });
});
