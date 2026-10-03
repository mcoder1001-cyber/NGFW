import { DesiredState } from '@ngfw/proto';
import { describe, expect, it } from 'vitest';
import { ValidationService } from '../../commit/validation.service.js';
import { parseDocument, redact } from '../../datastore/documents.js';

const configuration = () => parseDocument({
  management: {
    notifications: {
      channels: [{
        name: 'hook', type: 'webhook',
        webhook: { url: 'https://example.test/notify', secretRef: 'token/hook' },
      }],
      rules: [{ name: 'commits', events: ['commit'], channels: ['hook'] }],
    },
  },
});

describe('API-owned notification projection', () => {
  it('keeps the schema mirror roundtrippable while omitting delivery configuration from agent requests', () => {
    const document = configuration();
    const before = structuredClone(document);
    const mirrored = DesiredState.toJSON(DesiredState.fromJSON(redact(document))) as {
      management?: Record<string, unknown>;
    };
    // This assertion requires the additive notification proto mirror and prevents a vacuous
    // exclusion test where fromJSON silently drops a missing contract field.
    expect(mirrored.management?.['notifications']).toMatchObject({
      channels: [{ name: 'hook', type: 'webhook' }],
      rules: [{ name: 'commits', events: ['commit'] }],
    });
    const desired = DesiredState.toJSON(ValidationService.desiredState(document)) as {
      management?: Record<string, unknown>;
    };
    expect(desired.management).not.toHaveProperty('notifications');
    expect(desired.management).toHaveProperty('aaa');
    expect(document).toEqual(before);
  });
});
