import { status, type handleUnaryCall, type ServiceError } from '@grpc/grpc-js';
import type {
  DataplaneStartupPreviewRequest,
  DataplaneStartupPreviewResponse,
  DataplaneStartupStateRequest,
  DataplaneStartupStateResponse,
} from '@ngfw/proto';

type Json = Record<string, unknown>;

/** What the fake agent's "installed" startup.conf holds. */
export const FAKE_INSTALLED = 'cpu {\n  main-core 1\n  workers 2\n}\n';

function render(dp: Json): string {
  const cpu: string[] = [];
  if (typeof dp['mainCore'] === 'number') cpu.push(`  main-core ${dp['mainCore']}`);
  const corelist = dp['corelist'] as number[] | undefined;
  if (corelist && corelist.length > 0) cpu.push(`  corelist-workers ${corelist.join(',')}`);
  else if (typeof dp['workers'] === 'number') cpu.push(`  workers ${dp['workers']}`);
  return `cpu {\n${cpu.join('\n')}\n}\n`;
}

/**
 * F-dataplane-ui fakes of DataplaneStartupState / DataplaneStartupPreview for the in-process fake agent: the
 * installed file is FAKE_INSTALLED; the preview renders only the cpu section (the real renderer is Go,
 * internal/renderers/vppstartup) and a line diff. A corelist whose length differs from `workers` is refused as INVALID_ARGUMENT.
 */
export function dataplaneFake(): {
  dataplaneStartupState: handleUnaryCall<
    DataplaneStartupStateRequest,
    DataplaneStartupStateResponse
  >;
  dataplaneStartupPreview: handleUnaryCall<
    DataplaneStartupPreviewRequest,
    DataplaneStartupPreviewResponse
  >;
} {
  return {
    dataplaneStartupState: (_call, cb) =>
      cb(null, {
        runtimeThreads: [],
        loadedPlugins: '',
        nicQueues: '',
        runtimeMemory: '',
        runtimeErrors: ['vpp.disconnected'],
        startupPath: '/etc/vpp/startup.conf',
        startupPresent: true,
        workers: 2,
        corelistWorkers: '',
        mainCore: 1,
        plugins: { 'linux_cp_plugin.so': true },
        onlineCpus: '0-7',
        hugepagesTotalBytes: String(2 * 1024 ** 3),
        hugepagesFreeBytes: String(1024 ** 3),
        error: '',
        retrievedAt: new Date(0),
      }),
    dataplaneStartupPreview: (call, cb) => {
      const dp = (call.request.dataplane ?? {}) as unknown as Json;
      const cl = dp['corelist'] as number[] | undefined;
      if (typeof dp['workers'] === 'number' && cl && cl.length > 0 && cl.length !== dp['workers']) {
        cb({
          code: status.INVALID_ARGUMENT,
          details: 'vppstartup: input: dataplane.corelist: length differs from workers',
        } as ServiceError);
        return;
      }
      const rendered = render(dp);
      const a = FAKE_INSTALLED.split('\n');
      const b = rendered.split('\n');
      const diff =
        rendered === FAKE_INSTALLED
          ? ''
          : [
              '--- /etc/vpp/startup.conf',
              '+++ rendered',
              ...a.filter((l) => !b.includes(l)).map((l) => `-${l}`),
              ...b.filter((l) => !a.includes(l)).map((l) => `+${l}`),
            ].join('\n') + '\n';
      cb(null, {
        rendered,
        startupPath: '/etc/vpp/startup.conf',
        diff,
        changed: diff !== '',
        warnings: [],
        sha256: '0'.repeat(64),
      });
    },
  };
}
