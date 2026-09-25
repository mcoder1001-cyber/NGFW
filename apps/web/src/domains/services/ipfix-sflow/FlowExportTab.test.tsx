import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import { variantOf } from './FlowprobePanel';

/** F-ipfix-sflow: Services → Flow export against a scripted API (unit level). */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

function app(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  return (
    <App
      router={createTestRouter([path], { devRoutes: false })}
      streamUrl={STREAM}
      queryClient={queryClient}
    />
  );
}

const ipfix = {
  exporters: {
    lan: {
      enabled: true,
      collector: { address: '10.1.1.9', port: 3171 },
      sourceAddress: '10.1.1.1',
      vrf: 'default',
      pathMtu: 512,
      templateIntervalSec: 20,
      udpChecksum: false,
    },
  },
  flowprobe: {
    activeTimerSec: 15,
    passiveTimerSec: 120,
    recordL2: false,
    recordL3: true,
    recordL4: true,
    interfaces: [{ interface: 'host-w1a', direction: 'rx', l2: false, ip4: true, ip6: false }],
  },
  sflow: {
    enabled: true,
    samplingN: 1000,
    pollingIntervalSec: 20,
    headerBytes: 128,
    collectors: [{ address: '10.1.1.9', port: 3172 }],
    vrf: 'default',
    interfaces: ['host-w1b'],
  },
};

function withIpfix(api: FakeApi) {
  api.on('GET /api/v1/config/candidate/services', { body: { ipfix } });
  api.on('GET /api/v1/config/candidate/interfaces', {
    body: { 'host-w1a': {}, 'host-w1b': {}, 'host-w1c': {} },
  });
  api.on('GET /api/v1/state/ipfix', {
    body: {
      exporters: [
        {
          name: 'lan',
          defaultExporter: true,
          collector: '10.1.1.9',
          collectorPort: 3171,
          sourceAddress: '10.1.1.1',
          vrf: 'default',
          pathMtu: 512,
          templateIntervalSec: 20,
          udpChecksum: false,
          statIndex: null,
        },
      ],
      flowprobe: {
        params: {
          recordL2: false,
          recordL3: true,
          recordL4: true,
          activeTimerSec: 15,
          passiveTimerSec: 120,
        },
        interfaces: [{ interface: 'host-w1a', which: 'ip4', direction: 'rx' }],
      },
      sflow: {
        global: {
          samplingN: 1000,
          pollingIntervalSec: 20,
          headerBytes: 128,
          direction: 'rx',
          dropMonitoring: false,
        },
        interfaces: [{ interface: 'host-w1b', hwIfIndex: 7 }],
        counters: [{ name: '/err/sflow/sflow packets processed', value: '4242' }],
        exportsToCollectors: false,
      },
      globalsOwner: true,
      notes: [],
      retrievedAt: '2026-09-25T00:00:00.000Z',
    },
  });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('Services → Flow export', () => {
  it('variantOf: the schema default records ip4', () => {
    expect(variantOf({ interface: 'x' })).toBe('ip4');
    expect(variantOf({ interface: 'x', ip4: false, ip6: true })).toBe('ip6');
    expect(variantOf({ interface: 'x', ip4: false, ip6: false, l2: true })).toBe('l2');
  });

  it(
    'shows the hsflowd banner, exporters with live status, flowprobe and sFlow; saves through the services pointer route',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withIpfix(api);
      const patches: unknown[] = [];
      api.on('PATCH /api/v1/config/services', (_r, body) => {
        patches.push(body);
        return { body: { pointer: '/services', before: null, after: null } };
      });
      await signIn();
      render(app('/services?tab=flow-export'));
      expect(
        await screen.findByTestId('hsflowd-banner', {}, { timeout: 15_000 }),
      ).toHaveTextContent(/hsflowd/);
      const table = await screen.findByRole('table', { name: 'Exporters' });
      expect(await within(table).findByText('Exporter 0')).toBeInTheDocument();
      expect(within(table).getByText('10.1.1.9:3171')).toBeInTheDocument();

      fireEvent.click(screen.getByRole('button', { name: 'Remove lan' }));
      await waitFor(() => expect(patches).toEqual([{ ipfix: { exporters: { lan: null } } }]));

      fireEvent.click(screen.getByRole('tab', { name: 'Flowprobe' }));
      const fp = await screen.findByRole('table', { name: 'Monitored interfaces' });
      expect(within(fp).getByText('host-w1a')).toBeInTheDocument();
      fireEvent.click(screen.getByRole('button', { name: 'Save to candidate' }));
      await waitFor(() => expect(patches).toHaveLength(2));
      expect(patches[1]).toEqual({ ipfix: { flowprobe: ipfix.flowprobe } });

      fireEvent.click(screen.getByRole('tab', { name: 'sFlow' }));
      const live = await screen.findByRole('table', { name: 'Sampling on the data plane' });
      expect(within(live).getByText('4242')).toBeInTheDocument();
      fireEvent.click(screen.getByRole('button', { name: 'Save to candidate' }));
      await waitFor(() => expect(patches).toHaveLength(3));
      expect(patches[2]).toEqual({ ipfix: { sflow: { ...ipfix.sflow } } });
    },
  );
});
