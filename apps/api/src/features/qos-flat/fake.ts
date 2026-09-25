import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type {
  QosPolicerResetRequest,
  QosPolicerResetResponse,
  QosPolicerStateRequest,
  QosPolicerStateResponse,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

/** The fake agent's F-qos-flat RPCs (wave-A-hotspots P5): UNIMPLEMENTED until the feature fills them in. */
export function qosFlatFake(_agent: FakeAgent): {
  qosPolicerState: handleUnaryCall<QosPolicerStateRequest, QosPolicerStateResponse>;
  qosPolicerReset: handleUnaryCall<QosPolicerResetRequest, QosPolicerResetResponse>;
} {
  const unimplemented = (rpc: string) => (_call: unknown, cb: (err: unknown) => void) =>
    cb(Object.assign(new Error(`${rpc} is not implemented`), { code: status.UNIMPLEMENTED }));
  return {
    qosPolicerState: unimplemented('QosPolicerState'),
    qosPolicerReset: unimplemented('QosPolicerReset'),
  };
}
