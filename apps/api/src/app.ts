import 'reflect-metadata';
import fastifyCookie from '@fastify/cookie';
import fastifyWebsocket from '@fastify/websocket';
import { NestFactory } from '@nestjs/core';
import { FastifyAdapter, type NestFastifyApplication } from '@nestjs/platform-fastify';
import { DocumentBuilder, SwaggerModule, type OpenAPIObject } from '@nestjs/swagger';
import { RootConfig, ROOT_KEYS } from '@ngfw/schema';
import { z } from 'zod';
import type { FastifyInstance, RouteOptions } from 'fastify';
import { AppModule } from './app.module.js';
import { AuthService } from './auth/auth.service.js';
import { DOCS_COOKIE } from './auth/cookies.js';
import { loadEnv, type Env } from './config.js';
import { problems, toProblem } from './common/problem.js';
import { registerStreamRoute, STREAM_PROTOCOL } from './telemetry/stream.route.js';
import { RelayService } from './telemetry/relay.service.js';

export interface CreateAppOptions {
  env?: Env;
  logger?: false | ('error' | 'warn' | 'log' | 'debug')[];
  /** Observe every route as it is registered (the route-guard test enumerates them). */
  onRoute?: (route: RouteOptions) => void;
  /** Serve the OpenAPI UI/JSON at /api/docs (authenticated). Default true; the generator turns it off. */
  docs?: boolean;
}

export const DOCS_PATH = 'api/docs';

/** Body limit: a whole configuration document is well below this. */
const BODY_LIMIT = 8 * 1024 * 1024;

export async function createApp(opts: CreateAppOptions = {}): Promise<NestFastifyApplication> {
  const env = opts.env ?? loadEnv();
  const app = await NestFactory.create<NestFastifyApplication>(
    AppModule.forRoot(env),
    // TD-10b (review 2.3b): X-Forwarded-For/-Proto only from VRX_TRUST_PROXY peers (default loopback) — req.ip and
    // req.ips are the client behind the product nginx / the vite proxy, not the proxy (principal.ts sourceIp)
    new FastifyAdapter({
      bodyLimit: BODY_LIMIT,
      trustProxy: env.VRX_TRUST_PROXY.length > 0 ? env.VRX_TRUST_PROXY : false,
    }),
    { logger: opts.logger ?? ['error', 'warn', 'log'], abortOnError: false },
  );
  if (opts.onRoute) {
    (app.getHttpAdapter().getInstance() as unknown as FastifyInstance).addHook(
      'onRoute',
      opts.onRoute,
    );
  }
  await configureApp(app);
  if (opts.docs !== false) {
    // review L1: the API description is not public; same credentials as every other route. TD-10b (review 2.3g,
    // option (a)): a browser has no Authorization header, so a GET/HEAD here also accepts the session's docs cookie
    // (the access token, path /api/docs, set by login/refresh — auth/cookies.ts); the CLI keeps using the header
    const auth = app.get(AuthService);
    const fastify = app.getHttpAdapter().getInstance() as unknown as FastifyInstance;
    fastify.addHook('onRequest', async (req, reply) => {
      if (!req.url.startsWith(`/${DOCS_PATH}`)) return;
      const cookie = (req as { cookies?: Record<string, string | undefined> }).cookies?.[
        DOCS_COOKIE
      ];
      const header =
        req.headers.authorization ??
        (cookie !== undefined && (req.method === 'GET' || req.method === 'HEAD')
          ? `Bearer ${cookie}`
          : undefined);
      const p = await auth.authenticate(header).catch(() => null);
      if (p === null) {
        await reply
          .status(401)
          .header('content-type', 'application/problem+json')
          .header('www-authenticate', 'Bearer, ApiKey')
          .send(
            toProblem(
              problems.unauthorized(
                'authentication required: send Authorization, or log in to the web UI of this device first (its session opens the docs)',
              ),
              req.url.split('?')[0],
            ),
          );
      }
    });
    SwaggerModule.setup(DOCS_PATH, app, buildOpenApi(app));
  }
  return app;
}

/**
 * Fastify plumbing that Nest does not do: cookies (refresh token), JSON merge-patch bodies, the WebSocket relay route
 * and problem+json for errors raised outside Nest's pipeline (404 of unknown routes, body parse errors).
 */
export async function configureApp(app: NestFastifyApplication): Promise<void> {
  const fastify = app.getHttpAdapter().getInstance() as unknown as FastifyInstance;
  await app.register(fastifyCookie as never);
  const json = (
    _req: unknown,
    body: string,
    done: (err: Error | null, value?: unknown) => void,
  ) => {
    if (body === '') return done(null, undefined);
    try {
      done(null, JSON.parse(body));
    } catch (e) {
      done(
        Object.assign(new Error(`invalid JSON body: ${(e as Error).message}`), { statusCode: 400 }),
      );
    }
  };
  for (const type of ['application/merge-patch+json', 'application/problem+json']) {
    fastify.addContentTypeParser(type, { parseAs: 'string' }, json);
  }
  // Feature content-type parsers (SY2): register from the feature's own module first
  // (HttpAdapterHost in onModuleInit, before `ready`); a line here is the fallback only.
  // wave-BC: F-restconf-yang
  // wave-BC: F-backup-restore
  await app.register(fastifyWebsocket as never, {
    options: {
      maxPayload: 64 * 1024,
      handleProtocols: (protocols: Set<string>) =>
        protocols.has(STREAM_PROTOCOL) ? STREAM_PROTOCOL : false,
    },
  });
  await registerStreamRoute(fastify, app.get(AuthService), app.get(RelayService));
}

/** `interfaces` → `InterfacesConfig`: the component names packages/schema uses for its OpenAPI output. */
function componentName(key: string): string {
  return key.charAt(0).toUpperCase() + key.slice(1) + 'Config';
}

/** One domain as an OpenAPI 3.1 schema — the same Zod 4 conversion packages/schema's generator runs (D-006). */
function domainSchema(key: (typeof ROOT_KEYS)[number]): unknown {
  const js = z.toJSONSchema(RootConfig.shape[key], {
    target: 'draft-2020-12',
    io: 'input',
  }) as Record<string, unknown>;
  delete js['$schema'];
  return js;
}

/**
 * OpenAPI 3.1 from the Nest decorators plus the configuration schemas of packages/schema (00-CONTEXT rule 5: one
 * definition). `RootConfig` references the per-domain components instead of inlining them again.
 */
export function buildOpenApi(app: NestFastifyApplication): OpenAPIObject {
  const cfg = new DocumentBuilder()
    .setOpenAPIVersion('3.1.0')
    .setTitle('VRX API')
    .setDescription(
      [
        'Management API of the VRX secure router. `/config` is transactional (candidate → diff → commit → rollback),',
        '`/state` is live read-only, `/actions` are imperative. Errors are RFC 9457 `application/problem+json` with',
        '`errors[]{pointer,message}`. Telemetry: `WS /api/v1/stream` (subprotocols `vrx.v1, bearer.<token>`), messages',
        '`{subscribe:[topics]}`.',
      ].join(' '),
    )
    .setVersion('0.1.0')
    .setLicense('Proprietary', 'https://vrx.dev/license')
    .addServer('/', 'this device')
    .addBearerAuth({ type: 'http', scheme: 'bearer', bearerFormat: 'JWT' }, 'bearer')
    .addApiKey(
      {
        type: 'apiKey',
        in: 'header',
        name: 'Authorization',
        description: '`Authorization: ApiKey <key>`',
      },
      'apiKey',
    )
    .addCookieAuth(
      'vrx_refresh',
      { type: 'apiKey', in: 'cookie', name: 'vrx_refresh' },
      'refreshCookie',
    )
    .build();
  const doc = SwaggerModule.createDocument(app, cfg, {
    operationIdFactory: (controller, method) =>
      `${controller.replace(/Controller$/, '')}_${method}`,
  });
  const schemas: Record<string, unknown> = { ...(doc.components?.schemas ?? {}) };
  for (const key of ROOT_KEYS) schemas[componentName(key)] = domainSchema(key);
  schemas['RootConfig'] = {
    type: 'object',
    description:
      'The whole configuration document (docs/04). Secret leaves are write-only and never returned.',
    additionalProperties: false,
    properties: Object.fromEntries(
      ROOT_KEYS.map((k) => [k, { $ref: `#/components/schemas/${componentName(k)}` }]),
    ),
  };
  schemas['Problem'] = {
    type: 'object',
    description: 'RFC 9457 problem details',
    required: ['type', 'title', 'status'],
    properties: {
      type: { type: 'string' },
      title: { type: 'string' },
      status: { type: 'integer' },
      detail: { type: 'string' },
      instance: { type: 'string' },
      errors: {
        type: 'array',
        items: {
          type: 'object',
          required: ['pointer', 'message'],
          properties: {
            pointer: { type: 'string' },
            message: { type: 'string' },
            rule: { type: 'string' },
          },
        },
      },
    },
    additionalProperties: true,
  };
  doc.components = { ...doc.components, schemas: schemas as never };
  // routes without a security requirement are the reviewed @Public() ones: say so explicitly
  for (const item of Object.values(doc.paths)) {
    for (const op of Object.values(item) as { security?: unknown[]; operationId?: string }[]) {
      if (op && typeof op === 'object' && 'operationId' in op && op.security === undefined) {
        op.security = [];
      }
    }
  }
  return doc;
}
