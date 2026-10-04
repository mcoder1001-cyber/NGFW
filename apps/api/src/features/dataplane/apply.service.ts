import { Injectable } from '@nestjs/common';
import { request } from 'node:http';
import { z } from 'zod';
import { ProblemError } from '../../common/problem.js';

export const ApprovalOut = z.object({
  token: z.string().regex(/^[0-9a-f]{64}$/),
  sha256: z.string().regex(/^[0-9a-f]{64}$/),
  expiresIn: z.number().int().positive(),
});
export const ApplyOut = z.object({ accepted: z.literal(true), sha256: z.string() });

/** Only fixed Unix HTTP operations. The API never launches a process or writes root approval files. */
@Injectable()
export class DataplaneApplyService {
  async send(operation: '/approve' | '/apply', body: Record<string, unknown>): Promise<unknown> {
    return new Promise((resolve, reject) => {
      const data = JSON.stringify(body);
      const req = request(
        {
          socketPath: '/run/ngfw-apply/executor.sock',
          path: operation,
          method: 'POST',
          headers: {
            'content-type': 'application/json',
            'content-length': Buffer.byteLength(data),
          },
        },
        (res) => {
          let raw = '';
          res.setEncoding('utf8');
          res.on('data', (chunk: string) => {
            raw += chunk;
            if (raw.length > 4096) req.destroy(new Error('oversize executor response'));
          });
          res.on('error', () =>
            reject(new ProblemError(503, 'apply-unavailable', 'Apply executor unavailable')),
          );
          res.on('end', () => {
            try {
              const value: unknown = JSON.parse(raw);
              if (res.statusCode !== 200) {
                reject(
                  new ProblemError(
                    res.statusCode === 409 ? 409 : 503,
                    'apply-refused',
                    'Apply refused',
                    'Preview and approve again; inspect the appliance journal if the executor is unavailable.',
                  ),
                );
              } else resolve(value);
            } catch {
              reject(new ProblemError(503, 'apply-unavailable', 'Apply executor unavailable'));
            }
          });
        },
      );
      req.setTimeout(operation === '/apply' ? 1_210_000 : 20_000, () =>
        req.destroy(new Error('executor timeout')),
      );
      req.on('error', () =>
        reject(new ProblemError(503, 'apply-unavailable', 'Apply executor unavailable')),
      );
      req.end(data);
    });
  }
}
