import {
  status,
  type handleServerStreamingCall,
  type handleUnaryCall,
  type ServiceError,
} from '@grpc/grpc-js';
import type {
  CaptureChunk,
  CaptureDeleteRequest,
  CaptureDeleteResponse,
  CaptureFile,
  CaptureListRequest,
  CaptureListResponse,
  CaptureReadRequest,
} from '@ngfw/proto';
import { registerActionHandler, type ActionHandler } from '../../testing/fake-agent.js';
import { BPF_RE } from './dto.js';

/** A 1-packet little-endian pcap (the fake's "captured" file). */
export function fakePcap(): Buffer {
  const g = Buffer.alloc(24);
  g.writeUInt32LE(0xa1b2c3d4, 0);
  g.writeUInt16LE(2, 4);
  g.writeUInt16LE(4, 6);
  g.writeUInt32LE(65535, 16);
  g.writeUInt32LE(1, 20);
  const h = Buffer.alloc(16);
  h.writeUInt32LE(14, 8);
  h.writeUInt32LE(14, 12);
  return Buffer.concat([g, h, Buffer.alloc(14)]);
}

const TRACE_REASON = 'packet trace is not available on this build (D-128/TD-20, V18)';
const PG_REASON = 'packet-generator streams cannot be defined through the VPP binary API';

/**
 * F-capture-trace fakes of the capture action and CaptureList/Read/Delete: a capture "finishes" at once with a 1-packet
 * file unless `hold` is set (then it stays running → the next start is ABORTED/busy). Invalid BPF → INVALID_ARGUMENT
 * with the agent's "…: bpf: …" text.
 */
const state: { hold: boolean } = { hold: false };
const files = new Map<string, CaptureFile>();
let running: CaptureFile | undefined;
let n = 0;

/** Test-only: keep the next capture running (hold) and forget every file. */
export function resetCaptureFake(hold = false): void {
  state.hold = hold;
  files.clear();
  running = undefined;
}

const action: ActionHandler = (call) => {
  const a = call.request.capture!;
  if (!BPF_RE.test(a.bpf)) {
    call.emit('error', {
      code: status.INVALID_ARGUMENT,
      details: `invalid capture request: bpf: invalid character in expression`,
    } as ServiceError);
    return;
  }
  if (running) {
    call.emit('error', {
      code: status.ABORTED,
      details: 'busy: a pcap capture is already running',
    } as ServiceError);
    return;
  }
  const id = `fake-${++n}`;
  const f: CaptureFile = {
    id,
    state: 'running',
    interface: a.interface,
    direction: 'rx,tx',
    bpf: a.bpf,
    startedAt: new Date(0),
    stoppedAt: undefined,
    size: '0',
    packets: '0',
    sha256: '',
    maxPackets: a.maxPackets,
    seconds: a.seconds,
    snaplen: a.snaplen,
    reason: '',
  };
  call.write({ line: `capture ${id} started` });
  if (state.hold) {
    running = f;
    files.set(id, f);
    return;
  }
  files.set(id, {
    ...f,
    state: 'done',
    stoppedAt: new Date(1000),
    size: String(fakePcap().length),
    packets: '1',
    sha256: 'f'.repeat(64),
    reason: 'timeout',
  });
  call.write({ done: { summary: `capture ${id}: 1 packets`, exitCode: 0, stats: { id } } });
  call.end();
};

export function captureTraceFake() {
  registerActionHandler('capture', action);
  const notFound = (id: string) =>
    ({ code: status.NOT_FOUND, details: `no such capture: ${id}` }) as ServiceError;
  const captureList: handleUnaryCall<CaptureListRequest, CaptureListResponse> = (_c, cb) =>
    cb(null, {
      captures: [...files.values()].reverse(),
      maxFiles: 10,
      maxBytes: String(500 * 1024 * 1024),
      traceAvailable: false,
      traceReason: TRACE_REASON,
      pgAvailable: false,
      pgReason: PG_REASON,
    });
  const captureRead: handleServerStreamingCall<CaptureReadRequest, CaptureChunk> = (call) => {
    const f = files.get(call.request.id);
    if (!f || f.state === 'running') {
      call.emit('error', notFound(call.request.id));
      return;
    }
    call.write({ data: fakePcap() });
    call.end();
  };
  const captureDelete: handleUnaryCall<CaptureDeleteRequest, CaptureDeleteResponse> = (
    call,
    cb,
  ) => {
    const f = files.get(call.request.id);
    if (!f) return cb(notFound(call.request.id));
    if (f.state === 'running')
      return cb({
        code: status.FAILED_PRECONDITION,
        details: 'the capture is running',
      } as ServiceError);
    files.delete(call.request.id);
    cb(null, { size: f.size });
  };
  return { captureList, captureRead, captureDelete };
}
