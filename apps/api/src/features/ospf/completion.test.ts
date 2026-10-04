import { expect, it } from 'vitest';
import { RoutingStateResponse } from '@ngfw/proto';
import { ospfStateOut } from './state.js';
import { observations } from './observations.js';
import { OspfController } from './ospf.controller.js';
it('projects FRR v3 array neighbors', () => {
  const response = RoutingStateResponse.fromPartial({
    frrRunning: true,
    readers: {
      ospf6Neighbors: JSON.stringify({
        neighbors: [{ neighborId: '192.0.2.2', interfaceName: 'tap0', state: 'Full', priority: 1 }],
      }),
    },
  });
  expect(ospfStateOut(response, 'ospf6Neighbors').neighbors).toMatchObject([
    { routerId: '192.0.2.2', interface: 'tap0', state: 'Full' },
  ]);
  expect(ospfStateOut(response).unavailable).toBe('reader-unavailable');
});
it('paginates only public LSDB scalars and bounds malformed data', () => {
  const response = RoutingStateResponse.fromPartial({
    frrRunning: true,
    readers: {
      ospfDatabase: JSON.stringify({
        lsa: [
          { linkStateId: '192.0.2.1', age: 3, secret: 'sensitive' },
          { linkStateId: '192.0.2.2', age: 4, id: '1' },
        ],
      }),
    },
  });
  const projected = observations(response, 'ospfDatabase', 1, 1);
  expect(projected.total).toBe(2);
  expect(projected.rows).toEqual([{ linkStateId: '192.0.2.2', age: 4, id: '1' }]);
  expect(JSON.stringify(projected)).not.toContain('sensitive');
  response.readers.ospfDatabase = 'null';
  expect(observations(response, 'ospfDatabase', 0, 1).unavailable).toBe('reader-invalid');
  response.readers.ospfDatabase = 'x'.repeat(1024 * 1024 + 1);
  expect(observations(response, 'ospfDatabase', 0, 1).unavailable).toBe('reader-limit-exceeded');
});
it('rejects invalid query before RPC', async () => {
  const controller = new OspfController({
    routingState: () => {
      throw new Error('should not call');
    },
  } as never);
  await expect(controller.state('untrusted')).rejects.toThrow('version must be 2 or 3');
  await expect(controller.database('3', '-1', '101')).rejects.toThrow(
    'version must be 2 or 3; offset',
  );
});
