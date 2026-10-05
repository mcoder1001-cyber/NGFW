import { z } from 'zod';
import { objectName, secretRefOf } from '../../primitives.js';

export const bfdAuth = z
  .strictObject({
    type: z.enum(['keyed-sha1', 'meticulous-keyed-sha1']),
    keyId: z.number().int().min(0).max(255),
    keyRef: secretRefOf('key'),
  })
  .optional();
export const bfdMultihop = z.boolean().default(false);
export const BfdProfileSchema = z.strictObject({
  desiredMinTxUs: z.number().int().min(10000).max(60000000).default(300000),
  requiredMinRxUs: z.number().int().min(10000).max(60000000).default(300000),
  detectMultiplier: z.number().int().min(1).max(255).default(3),
});
export const bfdProfiles = z.record(objectName, BfdProfileSchema).default({});
export const bfdProfile = objectName.optional();
