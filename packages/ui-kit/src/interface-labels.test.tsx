import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { InterfaceLabelsProvider, useInterfaceLabel, useInterfaceName } from './interface-labels.js';
function Probe({ name }: { name: string }) {
  const label = useInterfaceLabel();
  const identity = useInterfaceName();
  return <output>{label(name)}|{identity(name)}</output>;
}
describe('interface display context', () => {
  it('preserves names without a provider', () => {
    render(<Probe name="loop0" />);
    expect(screen.getByRole('status')).toHaveTextContent('loop0|loop0');
  });
  it('resolves display aliases before conflicting raw names', () => {
    render(<InterfaceLabelsProvider labels={new Map([['wan', 'Ethernet1'], ['Ethernet1', 'Ethernet2']])}><Probe name="Ethernet1" /></InterfaceLabelsProvider>);
    expect(screen.getByRole('status')).toHaveTextContent('Ethernet2|wan');
  });
  it('keeps canonical identity, resolves candidate VLANs, and follows new labels', () => {
    const labels = new Map([['wan', 'Ethernet1'], ['GigabitEthernet0/8/0', 'Ethernet1'], ['wan.100', 'Ethernet1.100']]);
    const { rerender } = render(<InterfaceLabelsProvider labels={labels}><Probe name="Ethernet1.100.200" /></InterfaceLabelsProvider>);
    expect(screen.getByRole('status')).toHaveTextContent('Ethernet1.100.200|wan.100.200');
    rerender(<InterfaceLabelsProvider labels={labels}><Probe name="wan.200" /></InterfaceLabelsProvider>);
    expect(screen.getByRole('status')).toHaveTextContent('Ethernet1.200|wan.200');
    rerender(<InterfaceLabelsProvider labels={new Map([['wan', 'Ethernet2']])}><Probe name="wan.200" /></InterfaceLabelsProvider>);
    expect(screen.getByRole('status')).toHaveTextContent('Ethernet2.200|wan.200');
  });
});
