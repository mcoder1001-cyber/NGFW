import { NgfwThemeProvider } from '@ngfw/ui-kit';
import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import i18n from '../i18n';
import { DiffView, type DiffChange } from './DiffView';

afterEach(async () => {
  await i18n.changeLanguage('en');
});

describe('DiffView presentation', () => {
  it('shows neutral field and engine labels while retaining original change objects', () => {
    const changes: DiffChange[] = [
      { op: 'add', pointer: '/services/dns/vppCache', to: { enabled: true } },
      { op: 'replace', pointer: '/routing/static/0/viaFrr', from: false, to: true },
      {
        op: 'replace',
        pointer: '/vpn/ipsec/tunnels/branch/engine',
        from: 'strongswan',
        to: 'vpp-ikev2',
      },
      { op: 'add', pointer: '/ha/vrrp/gateway', to: { engine: 'vpp' } },
    ];
    const original = structuredClone(changes);
    const { container } = render(
      <NgfwThemeProvider mode="light" lang="en" dir="ltr">
        <DiffView changes={changes} />
      </NgfwThemeProvider>,
    );
    expect(container).not.toHaveTextContent(/frr|vpp|strongswan/i);
    expect(screen.getByText('/services/dns/dataplaneCache')).toBeInTheDocument();
    expect(screen.getByText('/routing/static/0/viaRoutingService')).toBeInTheDocument();
    expect(screen.getByText('"IPsec service"')).toBeInTheDocument();
    expect(screen.getByText('"Native IKEv2"')).toBeInTheDocument();
    expect(container).toHaveTextContent('Dataplane');
    expect(changes).toEqual(original);
  });

  it('keeps user descriptions, identifiers and redacted values faithful to the source', () => {
    const changes: DiffChange[] = [
      {
        op: 'add',
        pointer: '/interfaces/vpp~1frr',
        to: { description: 'strongSwan branch', unnumbered: 'vpp' },
      },
      {
        op: 'replace',
        pointer: '/system/passwordHash',
        from: 'must remain hidden',
        to: 'also hidden',
        redacted: true,
      },
    ];
    const { container } = render(
      <NgfwThemeProvider mode="light" lang="en" dir="ltr">
        <DiffView changes={changes} />
      </NgfwThemeProvider>,
    );
    expect(screen.getByText('/interfaces/vpp~1frr')).toBeInTheDocument();
    expect(container).toHaveTextContent('strongSwan branch');
    expect(container).toHaveTextContent('"unnumbered": "vpp"');
    expect(container).not.toHaveTextContent('must remain hidden');
    expect(container).not.toHaveTextContent('also hidden');
  });
});
