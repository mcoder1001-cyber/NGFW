import { readFile, stat, statfs } from 'node:fs/promises';
import { cpus, hostname, loadavg, totalmem, freemem, uptime } from 'node:os';

/**
 * WEB-dashboard host metrics (D-154): the appliance's own CPU, memory, disk and hugepages, read by vrx-api from the
 * kernel (`node:os`, `/proc/meminfo`, statfs) — no shell, no VPP. vrx-api and vrx-agent run on the same appliance (P10),
 * so the API's host is the router. Pure helpers here; the Nest service samples them on a timer.
 */

/** Cumulative CPU times of all cores (ms), from `os.cpus()`. */
export interface CpuTimes {
  busy: number;
  total: number;
}

export function cpuTimes(list = cpus()): CpuTimes {
  let busy = 0;
  let total = 0;
  for (const c of list) {
    const t = c.times;
    const all = t.user + t.nice + t.sys + t.idle + t.irq;
    total += all;
    busy += all - t.idle;
  }
  return { busy, total };
}

/** Utilisation between two samples, 0..100; undefined when there is nothing to compare. */
export function cpuPercent(prev: CpuTimes | undefined, cur: CpuTimes): number | undefined {
  if (!prev) return undefined;
  const dt = cur.total - prev.total;
  if (dt <= 0) return undefined;
  return Math.min(100, Math.max(0, ((cur.busy - prev.busy) / dt) * 100));
}

export interface MemInfo {
  totalBytes: number;
  availableBytes: number;
  hugepages: { total: number; free: number; sizeBytes: number } | null;
}

/** Parse `/proc/meminfo` (kB values); `MemAvailable` is what the kernel can give without swapping. */
export function parseMeminfo(text: string): MemInfo | undefined {
  const kv = new Map<string, number>();
  for (const line of text.split('\n')) {
    const m = /^([A-Za-z_()0-9]+):\s+(\d+)(?:\s+kB)?$/.exec(line.trim());
    if (m) kv.set(m[1]!, Number(m[2]));
  }
  const total = kv.get('MemTotal');
  const avail = kv.get('MemAvailable') ?? kv.get('MemFree');
  if (total === undefined || avail === undefined) return undefined;
  const hpTotal = kv.get('HugePages_Total') ?? 0;
  const hpSize = kv.get('Hugepagesize') ?? 0;
  return {
    totalBytes: total * 1024,
    availableBytes: avail * 1024,
    hugepages:
      hpTotal > 0 && hpSize > 0
        ? { total: hpTotal, free: kv.get('HugePages_Free') ?? 0, sizeBytes: hpSize * 1024 }
        : null,
  };
}

export async function memInfo(path = '/proc/meminfo'): Promise<MemInfo> {
  try {
    const parsed = parseMeminfo(await readFile(path, 'utf8'));
    if (parsed) return parsed;
  } catch {
    // not Linux (developer laptop): fall back to Node's view
  }
  return { totalBytes: totalmem(), availableBytes: freemem(), hugepages: null };
}

export interface DiskUsage {
  mount: string;
  totalBytes: number;
  usedBytes: number;
}

/** Usage of each mount point that exists, one entry per filesystem (paths on the same device are folded). */
export async function diskUsage(mounts: readonly string[]): Promise<DiskUsage[]> {
  const seen = new Set<string>();
  const out: DiskUsage[] = [];
  for (const mount of mounts) {
    try {
      const [st, fs] = await Promise.all([stat(mount), statfs(mount)]);
      const key = String(st.dev);
      if (seen.has(key)) continue;
      seen.add(key);
      const totalBytes = fs.blocks * fs.bsize;
      // `bavail` (what unprivileged writers can still use) — the figure `df` shows as available
      out.push({ mount, totalBytes, usedBytes: totalBytes - fs.bavail * fs.bsize });
    } catch {
      // mount point missing on this host
    }
  }
  return out;
}

export interface HostSample {
  /** ms since epoch */
  at: number;
  cpuPct: number | null;
  memUsedPct: number;
}

export interface HostSnapshot {
  hostname: string;
  uptimeSec: number;
  cpu: { cores: number; model: string; usagePct: number | null; load: [number, number, number] };
  memory: { totalBytes: number; usedBytes: number; availableBytes: number };
  hugepages: { total: number; free: number; sizeBytes: number } | null;
  disks: DiskUsage[];
  history: HostSample[];
  sampledAt: string;
}

export function snapshot(
  mem: MemInfo,
  disks: DiskUsage[],
  cpuPct: number | null,
  history: HostSample[],
  now: number,
): HostSnapshot {
  const list = cpus();
  const [l1 = 0, l5 = 0, l15 = 0] = loadavg();
  return {
    hostname: hostname(),
    uptimeSec: Math.round(uptime()),
    cpu: {
      cores: list.length,
      model: list[0]?.model?.trim() ?? '',
      usagePct: cpuPct,
      load: [l1, l5, l15],
    },
    memory: {
      totalBytes: mem.totalBytes,
      usedBytes: mem.totalBytes - mem.availableBytes,
      availableBytes: mem.availableBytes,
    },
    hugepages: mem.hugepages,
    disks,
    history,
    sampledAt: new Date(now).toISOString(),
  };
}
