import { canonicalSource, parseIp, isAllowlisted, type ParsedPrefix } from './engine.js';
/** Serializes complete snapshots: a slow RPC can never resurrect an older set. */
export class SnapshotPublisher {
  private dirty = false;
  private running: Promise<void> | undefined;
  private stopped = false;

  constructor(
    private readonly push: () => Promise<void>,
    private readonly failed: (e: unknown) => void,
  ) {}

  request(): void {
    if (this.stopped) return;
    this.dirty = true;
    this.flush();
  }

  stop(): void {
    this.stopped = true;
  }

  private flush(): void {
    if (this.running || this.stopped) return;
    this.running = this.run().finally(() => {
      this.running = undefined;
    });
  }

  private async run(): Promise<void> {
    while (this.dirty && !this.stopped) {
      this.dirty = false;
      try {
        await this.push();
      } catch (e) {
        this.dirty = true;
        this.failed(e);
        return;
      }
    }
  }
}

/** Database keys are ipCidr host prefixes; the runtime RPC requires bare addresses. */
export function runtimeSources(
  entries: { source: string; expiresAt: string }[],
  allow: ParsedPrefix[],
): { source: string; expiresAt: Date }[] {
  const sources = new Map<string, { source: string; expiresAt: Date }>();
  for (const entry of entries) {
    const [address, bits, extra] = entry.source.split('/');
    if (!address || extra !== undefined) continue;
    const parsed = parseIp(address);
    if (!parsed || Number(bits) !== (parsed.family === 4 ? 32 : 128)) continue;
    const canonical = canonicalSource(address);
    if (!canonical || isAllowlisted(allow, address)) continue;
    const source = canonical.split('/')[0]!;
    const expiresAt = new Date(entry.expiresAt);
    if (!Number.isFinite(expiresAt.getTime())) continue;
    const old = sources.get(source);
    if (!old || old.expiresAt < expiresAt) sources.set(source, { source, expiresAt });
  }
  return [...sources.values()];
}
