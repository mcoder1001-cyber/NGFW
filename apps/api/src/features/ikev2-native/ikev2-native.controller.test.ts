import 'reflect-metadata';
import { describe, expect, it, vi } from 'vitest';
import type { AgentClient } from '../../agent/agent.client.js';
import { ROLE_KEY } from '../../auth/decorators.js';
import { Ikev2NativeController, NativeIkeActionInput } from './ikev2-native.controller.js';

describe('native IKE runtime actions', () => {
  it('requires admin and preserves an unsigned 64-bit SPI without rounding', async () => {
    expect(Reflect.getMetadata(ROLE_KEY, Ikev2NativeController.prototype.action)).toBe('admin');
    const runAction = vi.fn().mockResolvedValue({ lines: [], done: { exitCode: 0 } });
    const controller = new Ikev2NativeController({ runAction } as unknown as AgentClient);
    await controller.action(
      'site',
      'delete-sa',
      NativeIkeActionInput.parse({ ikeSpi: '18446744073709551615' }),
    );
    expect(runAction).toHaveBeenCalledWith({
      ikev2: {
        tunnel: 'site',
        operation: 'delete-sa',
        ikeSpi: '18446744073709551615',
        childSpi: 0,
      },
    });
  });
  it('rejects overflow, signed SPIs and unsolicited configuration/key material', () => {
    for (const body of [
      { ikeSpi: '18446744073709551616' },
      { ikeSpi: '-1' },
      { childSpi: 4294967296 },
      { secret: 'material' },
      { tunnel: 'foreign' },
    ]) {
      expect(NativeIkeActionInput.safeParse(body).success).toBe(false);
    }
  });
});
