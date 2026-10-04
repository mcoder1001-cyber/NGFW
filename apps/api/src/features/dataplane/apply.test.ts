import { describe, expect, it, vi } from 'vitest';
import type { AgentClient } from '../../agent/agent.client.js';
import type { NgfwRequest } from '../../common/principal.js';
import type { DatastoreService } from '../../datastore/datastore.service.js';
import { DataplaneApplyController } from './apply.controller.js';
import type { DataplaneApplyService } from './apply.service.js';

const sha256 = 'a'.repeat(64);
function fixture(changed = true, digest = sha256) {
  const send = vi.fn(async (op: string) =>
    op === '/approve'
      ? { token: 'b'.repeat(64), sha256, expiresIn: 120 }
      : { accepted: true, sha256 },
  );
  const controller = new DataplaneApplyController(
    {
      dataplaneStartupPreview: vi.fn(async () => ({ changed, sha256: digest })),
    } as unknown as AgentClient,
    {
      getCandidate: vi.fn(async () => ({
        dataplane: { workers: 4 },
        system: { privateValue: 'unrelated' },
      })),
    } as unknown as DatastoreService,
    { send } as unknown as DataplaneApplyService,
  );
  const req = { principal: { id: 7 } } as NgfwRequest;
  return { controller, req, send };
}

describe('approved dataplane action routing', () => {
  it('derives actor from authenticated principal, scopes document and never audits the token', async () => {
    const { controller, req, send } = fixture();
    const approval = await controller.approve(req, { sha256 });
    await controller.apply(req, { sha256, token: approval.token });
    expect(send.mock.calls).toEqual([
      ['/approve', { actor: '7', sha256, dataplane: { workers: 4 } }],
      ['/apply', { actor: '7', sha256, dataplane: { workers: 4 }, token: approval.token }],
    ]);
    expect(req.audit).toEqual({ resource: '/dataplane/startup', after: { sha256 } });
  });
  it.each([
    [false, sha256],
    [true, 'c'.repeat(64)],
  ])('refuses unchanged or stale preview before root issuance', async (changed, digest) => {
    const { controller, req, send } = fixture(changed as boolean, digest as string);
    await expect(controller.approve(req, { sha256 })).rejects.toMatchObject({
      slug: 'preview-changed',
    });
    expect(send).not.toHaveBeenCalled();
  });
});
