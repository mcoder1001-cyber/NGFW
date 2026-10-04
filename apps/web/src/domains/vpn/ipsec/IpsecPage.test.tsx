import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';
import {
  localizeIpsecSchema,
  proposalFormSchema,
  tunnelChip,
  tunnelChipState,
  tunnelFormSchema,
  type IpsecTunnelState,
} from './model';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

type Props = Record<
  string,
  {
    title?: string;
    'x-ngfw-ui'?: { help?: string; group?: string };
    properties?: Props;
    anyOf?: { properties?: Props }[];
    oneOf?: { properties?: Props }[];
  }
>;

describe('IPsec model', () => {
  it('creates only native route-based tunnels with a required protected interface', () => {
    const form = tunnelFormSchema();
    const engine = (
      form.properties as Record<string, { default?: string; const?: string; enum?: string[] }>
    )['engine'];
    expect(engine?.default).toBe('vpp-ikev2');
    expect(engine?.const ?? engine?.enum).toEqual(engine?.const ? 'vpp-ikev2' : ['vpp-ikev2']);
    expect(form.required).toContain('routeBased');
    const auth = (
      form.properties as Record<
        string,
        {
          oneOf?: { properties?: Record<string, { const?: string }> }[];
          anyOf?: { properties?: Record<string, { const?: string }> }[];
        }
      >
    )['auth'];
    expect((auth?.oneOf ?? auth?.anyOf)?.map((branch) => branch.properties?.method?.const)).toEqual(
      ['psk', 'cert'],
    );
    const cert = (auth?.oneOf ?? auth?.anyOf)?.find(
      (branch) => branch.properties?.method?.const === 'cert',
    ) as { required?: string[]; properties?: Record<string, unknown> };
    expect(cert.required).toContain('peerCertificate');
    expect(cert.properties).not.toHaveProperty('remoteCa');
  });

  it('localizes every field, nested groups and union branches; schema help never leaks (R6 M1)', () => {
    const fa = i18n.getFixedT('fa', 'ipsec');
    const en = i18n.getFixedT('en', 'ipsec');
    const tunnel = localizeIpsecSchema(tunnelFormSchema(), (k, o) => fa(k, o ?? {}));
    const props = tunnel.properties as Props;
    expect(props['localAddr']?.title).toBe(fa('field.localAddr.title'));
    expect(props['rekey']?.properties?.['espSec']?.title).toBe(fa('field.rekey.espSec.title'));
    const branches = [...(props['auth']?.anyOf ?? []), ...(props['auth']?.oneOf ?? [])];
    const branchTitles = branches.flatMap((b) =>
      Object.values(b.properties ?? {}).map((p) => p.title),
    );
    expect(branchTitles).toContain(fa('field.auth.secretRef.title'));
    // every property has an fa title key, and any help shown is the fa translation (never the schema's English)
    const walk = (p: Props | undefined, path: string): void => {
      for (const [k, v] of Object.entries(p ?? {})) {
        const key = path ? `${path}.${k}` : k;
        expect(i18n.exists(`ipsec:field.${key}.title`, { lng: 'fa' }), key).toBe(true);
        expect(v.title, key).toBe(fa(`field.${key}.title`));
        if (v['x-ngfw-ui']?.help) expect(v['x-ngfw-ui'].help, key).toBe(fa(`field.${key}.help`));
        walk(v.properties, key);
      }
    };
    walk(props, '');
    expect(en('field.engine.help')).not.toMatch(/kernel-vpp|D6\.3/);
    const proposal = localizeIpsecSchema(proposalFormSchema(), (k, o) => fa(k, o ?? {}));
    expect((proposal.properties as Props)['ike']?.properties?.['dh']?.title).toBe(
      fa('field.ike.dh.title'),
    );
  });

  it('shows a neutral chip while state is unknown or the tunnel is not applied (R6 M3)', () => {
    const up = { status: 'up' } as IpsecTunnelState;
    expect(tunnelChipState(false, up)).toBe('unknown');
    expect(tunnelChipState(true, undefined)).toBe('notApplied');
    expect(tunnelChipState(true, up)).toBe('up');
    expect([
      tunnelChip('unknown'),
      tunnelChip('notApplied'),
      tunnelChip('connecting'),
      tunnelChip('down'),
    ]).toEqual(['adminDown', 'adminDown', 'degraded', 'down']);
  });
});

describe('IpsecPage', () => {
  // R6 B1: WireGuard and IPsec both read the vpn candidate; each tab must keep its own cache entry.
  it('keeps its candidate apart from the WireGuard tab on one QueryClient, and confirms deletes', async () => {
    const api = installFakeApi();
    const candidate = {
      wireguard: { interfaces: {} },
      ipsec: {
        proposals: {
          p1: {
            ike: { encr: 'aes256gcm16', prf: 'prfsha256', dh: 'curve25519' },
            esp: { encr: 'aes256gcm16' },
          },
        },
        tunnels: {
          'site-a': {
            localAddr: '10.2.250.1',
            remoteAddr: '10.2.250.2',
            proposal: 'p1',
            ikeVersion: 2,
          },
        },
      },
    };
    api.on('GET /api/v1/config/candidate/vpn', { body: candidate });
    api.on('GET /api/v1/state/vpn/wireguard', {
      body: { retrievedAt: null, eventsActive: false, interfaces: [] },
    });
    api.on('GET /api/v1/state/ipsec/tunnels', {
      body: {
        retrievedAt: null,
        eventsActive: false,
        charonRestarted: false,
        daemonVersion: '5.9.6',
        pendingAction: '',
        tunnels: [],
      },
    });
    await signIn();
    // App's staleTime (App.tsx): with staleTime 0 every mount refetches and a shared key would go unnoticed
    // (verify r1 #1). With 5 s the IPsec tab reuses whatever the WireGuard tab cached under the same key.
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 5_000 } },
    });
    render(
      <App
        router={createTestRouter(['/vpn?tab=wireguard'], { devRoutes: false })}
        streamUrl={STREAM}
        queryClient={queryClient}
      />,
    );
    await screen.findByText('No WireGuard interface is configured.', {}, { timeout: 15_000 });
    fireEvent.click(screen.getByRole('tab', { name: 'IPsec' }));
    const row = await screen.findByTestId('ipsec-tunnel-site-a', {}, { timeout: 15_000 });
    expect(within(row).getByText('Not applied')).toBeInTheDocument();
    expect(screen.getByTestId('ipsec-proposal-p1')).toBeInTheDocument();
    // one GET of the vpn candidate per cache entry (WireGuard's and IPsec's), no more
    const candidateGets = () =>
      api.calls.filter((c) => c.method === 'GET' && c.path === '/api/v1/config/candidate/vpn')
        .length;
    expect(candidateGets()).toBe(2);

    fireEvent.click(within(row).getByRole('button', { name: 'Delete' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(/Tunnel site-a is removed/)).toBeInTheDocument();
    expect(api.calls.some((c) => c.method === 'PATCH')).toBe(false); // nothing sent before confirming
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));

    fireEvent.click(screen.getByRole('tab', { name: 'WireGuard' }));
    expect(await screen.findByText('No WireGuard interface is configured.')).toBeInTheDocument();
  });
  // verify r1 #2 (D-155): the agent's ActionRequired text is English and names the engine; the UI shows only a
  // fixed translated message.
  it('shows a fixed translated restart notice, never the agent text', async () => {
    const raw =
      'strongswan: charon must be restarted (strongswan): strongswan.conf loads plugins charon is not running: kernel-vpp';
    const api = installFakeApi();
    api.on('GET /api/v1/config/candidate/vpn', { body: { ipsec: { proposals: {}, tunnels: {} } } });
    api.on('GET /api/v1/state/ipsec/tunnels', {
      body: {
        retrievedAt: null,
        eventsActive: false,
        charonRestarted: false,
        daemonVersion: '5.9.6',
        pendingAction: raw,
        tunnels: [],
      },
    });
    await signIn();
    render(
      <App
        router={createTestRouter(['/vpn?tab=ipsec'], { devRoutes: false })}
        streamUrl={STREAM}
        queryClient={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      />,
    );
    const alert = await screen.findByTestId('ipsec-pending-restart', {}, { timeout: 15_000 });
    expect(alert).toHaveTextContent(
      'The IKE daemon must be restarted to load the new configuration.',
    );
    expect(document.body.textContent).not.toMatch(/kernel-vpp|charon|strongswan\.conf/);
    await i18n.changeLanguage('fa');
    const fa = i18n.getFixedT('fa', 'ipsec')('pendingRestart');
    expect(fa).not.toBe('pendingRestart');
    expect(await screen.findByText(fa)).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/kernel-vpp|charon|strongswan\.conf/);
  });
});
