import type { DataplaneStartupPreviewResponse, DataplaneStartupStateResponse } from '@ngfw/proto';
import { describe, expect, it } from 'vitest';
import {
  DataplanePreviewOut,
  DataplaneStateOut,
  toDataplanePreview,
  toDataplaneState,
} from './dataplane.controller.js';
import { dataplaneFake } from './fake.js';

const fake = dataplaneFake();

function state(): Promise<DataplaneStartupStateResponse> {
  return new Promise((resolve, reject) =>
    fake.dataplaneStartupState({} as never, (err, res) => (err ? reject(err) : resolve(res!))),
  );
}
function preview(dataplane: Record<string, unknown>): Promise<DataplaneStartupPreviewResponse> {
  return new Promise((resolve, reject) =>
    fake.dataplaneStartupPreview({ request: { dataplane } } as never, (err, res) =>
      err ? reject(err) : resolve(res!),
    ),
  );
}

describe('F-dataplane-ui state + preview', () => {
  it('maps the state RPC to the documented shape (uint64 as strings)', async () => {
    const out = toDataplaneState(await state());
    expect(DataplaneStateOut.parse(out)).toEqual(out);
    expect(out).toMatchObject({ workers: 2, mainCore: 1, hugepagesTotalBytes: '2147483648' });
  });

  it('preview of a candidate that changes workers carries the diff and the restart flags', async () => {
    const out = toDataplanePreview(await preview({ workers: 4, mainCore: 1 }));
    expect(DataplanePreviewOut.parse(out)).toEqual(out);
    expect(out.changed).toBe(true);
    expect(out.diff).toContain('-  workers 2');
    expect(out.diff).toContain('+  workers 4');
    expect(out).toMatchObject({ restartRequired: true, applyAvailable: false });
  });

  it('an unchanged candidate has no diff', async () => {
    const out = toDataplanePreview(await preview({ workers: 2, mainCore: 1 }));
    expect(out).toMatchObject({ changed: false, diff: '' });
  });
});
