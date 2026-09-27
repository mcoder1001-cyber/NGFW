import { CaptureDirection, type CaptureAction, type CaptureListResponse } from '@ngfw/proto';
import { z } from 'zod';

/** pcap-filter(7) alphabet, the same set the agent enforces (descriptors/trace BPFFilter.Validate): no quotes, `;`, `$`, `\`. */
export const BPF_RE = /^[A-Za-z0-9 .:/[\]()&|!=<>\-+*%^~_,]*$/;

export const CaptureBody = z
  .object({
    interface: z
      .string()
      .min(1)
      .max(63)
      .regex(/^[A-Za-z0-9._/:-]+$/)
      .describe('VPP interface name; "any" = every interface'),
    direction: z.enum(['rx', 'tx', 'both']).default('both'),
    drop: z.boolean().default(false).describe('also capture dropped packets'),
    errorFilter: z
      .string()
      .max(127)
      .regex(/^[A-Za-z0-9._-]+\/[A-Za-z0-9._-]+$/)
      .optional()
      .describe('drop capture limited to one "node/error" counter (needs drop)'),
    bpf: z
      .string()
      .max(1024)
      .regex(BPF_RE, 'only the pcap-filter alphabet: no quotes, ";", "$" or backslash')
      .default('')
      .describe('pcap-filter(7) expression (bpf_trace_filter plugin; globals-owner agent only)'),
    maxPackets: z.number().int().min(1).max(100000).default(1000),
    seconds: z.number().int().min(1).max(600).default(30),
    snaplen: z.number().int().min(32).max(9000).default(9000),
  })
  .strict()
  .refine((b) => b.errorFilter === undefined || b.drop, {
    path: ['errorFilter'],
    message: 'needs drop: true',
  })
  .describe('Start one pcap capture (one per VPP)');

export const CaptureStarted = z.object({ id: z.string() });

export const CaptureFileOut = z.object({
  id: z.string(),
  state: z.enum(['running', 'done', 'empty', 'interrupted']).or(z.string()),
  interface: z.string(),
  direction: z.string().describe('"rx", "tx", "drop" joined by ","'),
  bpf: z.string(),
  startedAt: z.string().nullable(),
  stoppedAt: z.string().nullable(),
  size: z.string().describe('bytes, uint64 as decimal string (D-039)'),
  packets: z.string().describe('uint64 as decimal string (D-039)'),
  sha256: z.string(),
  maxPackets: z.number().int(),
  seconds: z.number().int(),
  snaplen: z.number().int(),
  reason: z.string(),
});

export const CapturesOut = z
  .object({
    captures: z.array(CaptureFileOut),
    maxFiles: z.number().int(),
    maxBytes: z.string(),
    trace: z.object({ available: z.boolean(), reason: z.string() }),
    pg: z.object({ available: z.boolean(), reason: z.string() }),
  })
  .describe(
    'pcap files kept by the agent (0600, retention caps) + the running capture; trace / PG availability',
  );

const DIR: Record<string, CaptureDirection> = {
  rx: CaptureDirection.CAPTURE_DIRECTION_RX,
  tx: CaptureDirection.CAPTURE_DIRECTION_TX,
  both: CaptureDirection.CAPTURE_DIRECTION_BOTH,
};

export function toCaptureAction(b: z.output<typeof CaptureBody>): CaptureAction {
  return {
    interface: b.interface,
    bpf: b.bpf,
    maxPackets: b.maxPackets,
    seconds: b.seconds,
    // drop alone (no rx/tx wanted) is not expressible in the form: rx/tx/both + optional drop
    direction: DIR[b.direction] ?? CaptureDirection.CAPTURE_DIRECTION_BOTH,
    snaplen: b.snaplen,
    drop: b.drop,
    errorFilter: b.errorFilter ?? '',
  };
}

const ts = (d: Date | undefined) => (d ? new Date(d).toISOString() : null);

export function toCaptures(r: CaptureListResponse): z.infer<typeof CapturesOut> {
  return {
    captures: r.captures.map((c) => ({
      id: c.id,
      state: c.state,
      interface: c.interface,
      direction: c.direction,
      bpf: c.bpf,
      startedAt: ts(c.startedAt),
      stoppedAt: ts(c.stoppedAt),
      size: String(c.size ?? '0'),
      packets: String(c.packets ?? '0'),
      sha256: c.sha256,
      maxPackets: c.maxPackets,
      seconds: c.seconds,
      snaplen: c.snaplen,
      reason: c.reason,
    })),
    maxFiles: r.maxFiles,
    maxBytes: String(r.maxBytes ?? '0'),
    trace: { available: r.traceAvailable, reason: r.traceReason },
    pg: { available: r.pgAvailable, reason: r.pgReason },
  };
}

/** Agent INVALID_ARGUMENT text "…: <field>: <detail>" → the body pointer. */
export function pointerOf(detail: string): string {
  const m = /invalid capture request: (\w+): /.exec(detail);
  return m?.[1] ? `/${m[1]}` : '';
}
