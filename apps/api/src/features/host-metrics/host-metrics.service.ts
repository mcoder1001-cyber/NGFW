import { Injectable, type OnModuleDestroy, type OnModuleInit } from '@nestjs/common';
import {
  cpuPercent,
  cpuTimes,
  diskUsage,
  memInfo,
  snapshot,
  type CpuTimes,
  type HostSample,
  type HostSnapshot,
} from './host-metrics.js';

/** Sample period and history length: 5 s × 72 = the last 6 minutes, so the dashboard charts are full on first load. */
export const SAMPLE_MS = 5_000;
export const HISTORY = 72;
/** Filesystems shown on the dashboard (folded when they share a device). */
export const MOUNTS = ['/', '/var', '/var/log'] as const;

@Injectable()
export class HostMetricsService implements OnModuleInit, OnModuleDestroy {
  private timer: NodeJS.Timeout | undefined;
  private prev: CpuTimes | undefined;
  private lastPct: number | null = null;
  private readonly history: HostSample[] = [];

  onModuleInit(): void {
    this.prev = cpuTimes();
    this.timer = setInterval(() => void this.sample().catch(() => undefined), SAMPLE_MS);
    this.timer.unref();
  }

  onModuleDestroy(): void {
    if (this.timer) clearInterval(this.timer);
  }

  /** One timer tick: CPU over the last period + memory, appended to the ring. */
  async sample(now = Date.now()): Promise<void> {
    const cur = cpuTimes();
    this.lastPct = cpuPercent(this.prev, cur) ?? this.lastPct;
    this.prev = cur;
    const mem = await memInfo();
    this.history.push({
      at: now,
      cpuPct: this.lastPct === null ? null : Math.round(this.lastPct * 10) / 10,
      memUsedPct:
        Math.round(((mem.totalBytes - mem.availableBytes) / Math.max(mem.totalBytes, 1)) * 1000) /
        10,
    });
    if (this.history.length > HISTORY) this.history.splice(0, this.history.length - HISTORY);
  }

  async current(now = Date.now()): Promise<HostSnapshot> {
    const [mem, disks] = await Promise.all([memInfo(), diskUsage(MOUNTS)]);
    return snapshot(mem, disks, this.lastPct, [...this.history], now);
  }
}
