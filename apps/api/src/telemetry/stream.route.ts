import type { FastifyInstance, FastifyReply, FastifyRequest } from 'fastify';
import type { WebSocket } from 'ws';
import type { AuthService } from '../auth/auth.service.js';
import { problems, toProblem } from '../common/problem.js';
import type { VrxRequest } from '../common/principal.js';
import type { RelayService } from './relay.service.js';

export const STREAM_PATH = '/api/v1/stream';
export const STREAM_PROTOCOL = 'vrx.v1';
const BEARER_PROTOCOL = /^bearer\.([A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)$/;

/**
 * Browsers cannot set headers on a WebSocket: they offer the subprotocols `vrx.v1, bearer.<access-token>`. Other
 * clients may send `Authorization: Bearer …` / `ApiKey …`. The token never goes into the URL (URLs end up in logs).
 */
export function streamCredential(req: FastifyRequest): string | undefined {
  if (req.headers.authorization) return req.headers.authorization;
  const offered = (req.headers['sec-websocket-protocol'] ?? '').split(',').map((s) => s.trim());
  for (const p of offered) {
    const m = BEARER_PROTOCOL.exec(p);
    if (m) return `Bearer ${m[1]}`;
  }
  return undefined;
}

/**
 * `WS /api/v1/stream` — registered on Fastify directly (Nest gateways do not run the HTTP guard), so it authenticates
 * itself in `preValidation`, before the upgrade: no credential → 401 problem+json, like every HTTP route.
 */
export async function registerStreamRoute(
  fastify: FastifyInstance,
  auth: AuthService,
  relay: RelayService,
): Promise<void> {
  await fastify.register(async (scope) => {
    scope.get(
      STREAM_PATH,
      {
        websocket: true,
        preValidation: async (req: FastifyRequest, reply: FastifyReply) => {
          try {
            const principal = await auth.authenticate(streamCredential(req));
            if (principal === null) throw problems.unauthorized();
            (req as VrxRequest).principal = principal;
          } catch (e) {
            const body = toProblem(e, STREAM_PATH);
            await reply
              .status(body.status)
              .header('content-type', 'application/problem+json')
              .header('www-authenticate', 'Bearer, ApiKey')
              .send(body);
          }
        },
      },
      (socket: WebSocket, req: FastifyRequest) => {
        relay.attach(socket, (req as VrxRequest).principal!);
      },
    );
  });
}
