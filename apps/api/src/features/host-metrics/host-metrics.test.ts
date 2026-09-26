import { describe, expect, it } from 'vitest';
import { cpuPercent, diskUsage, memInfo, parseMeminfo } from './host-metrics.js';
import { HISTORY, HostMetricsService } from './host-metrics.service.js';

const MEMINFO = `MemTotal:       16318864 kB
MemFree:         1203456 kB
MemAvailable:    9876543 kB
Buffers:          123456 kB
HugePages_Total:     555
HugePages_Free:      300
Hugepagesize:       2048 kB
`;

describe('host metrics (WEB-dashboard, D-154)', () => {
  it('parses /proc/meminfo: MemAvailable, and hugepages when reserved', () => {
    expect(parseMeminfo(MEMINFO)).toEqual({
      totalBytes: 16318864 * 1024,
      availableBytes: 9876543 * 1024,
      hugepages: { total: 555, free: 300, sizeBytes: 2048 * 1024 },
    });
    expect(parseMeminfo('MemTotal: 100 kB\nMemFree: 40 kB\n')).toEqual({
      totalBytes: 102400,
      availableBytes: 40960,
      hugepages: null,
    });
    expect(parseMeminfo('garbage')).toBeUndefined();
  });

  it('falls back to os memory when /proc/meminfo is missing', async () => {
    const m = await memInfo('/nonexistent/meminfo');
    expect(m.totalBytes).toBeGreaterThan(0);
    expect(m.hugepages).toBeNull();
  });

  it('computes CPU utilisation between two samples', () => {
    expect(cpuPercent(undefined, { busy: 1, total: 2 })).toBeUndefined();
    expect(cpuPercent({ busy: 100, total: 1000 }, { busy: 350, total: 2000 })).toBe(25);
    expect(cpuPercent({ busy: 100, total: 1000 }, { busy: 100, total: 1000 })).toBeUndefined();
  });

  it('reads disk usage once per filesystem and skips missing mounts', async () => {
    const d = await diskUsage(['/', '/', '/definitely/not/here']);
    expect(d).toHaveLength(1);
    expect(d[0]!.mount).toBe('/');
    expect(d[0]!.totalBytes).toBeGreaterThan(0);
    expect(d[0]!.usedBytes).toBeGreaterThanOrEqual(0);
  });

  it('keeps a bounded history and reports a snapshot', async () => {
    const s = new HostMetricsService();
    s.onModuleInit();
    try {
      for (let i = 0; i < HISTORY + 5; i += 1) await s.sample(1_000 * i);
      const snap = await s.current(1_000_000);
      expect(snap.history).toHaveLength(HISTORY);
      expect(snap.history[0]!.at).toBe(5_000);
      expect(snap.cpu.cores).toBeGreaterThan(0);
      expect(snap.memory.usedBytes + snap.memory.availableBytes).toBe(snap.memory.totalBytes);
      expect(snap.disks.length).toBeGreaterThan(0);
    } finally {
      s.onModuleDestroy();
    }
  });
});
