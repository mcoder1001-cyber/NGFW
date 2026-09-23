import { z } from 'zod';

/** All runtime configuration comes from VRX_* environment variables, validated once at boot. */
const Env = z.object({
  VRX_HTTP_HOST: z.string().default('127.0.0.1'),
  VRX_HTTP_PORT: z.coerce.number().int().min(1).max(65535).default(3000),
  VRX_AGENT_SOCKET: z.string().default('/run/vrx/agent.sock'),
  VRX_LOG_LEVEL: z.enum(['debug', 'info', 'warn', 'error']).default('info'),
});
export type Env = z.infer<typeof Env>;

export function loadEnv(source: NodeJS.ProcessEnv = process.env): Env {
  const r = Env.safeParse(source);
  if (!r.success) {
    throw new Error(`invalid environment: ${JSON.stringify(r.error.issues)}`);
  }
  return r.data;
}
