import { z } from 'zod';

/**
 * Root configuration document. Every top-level key is optional-with-default so that an
 * empty document is valid. Domain models are filled in by task P02; until then they are
 * passthrough placeholders. Changing this package requires a PR labelled `contract`.
 */
export const RootConfig = z
  .object({
    system: z.object({}).passthrough().default({}), // TODO(P02)
    dataplane: z.object({}).passthrough().default({}), // TODO(P02)
    interfaces: z.object({}).passthrough().default({}), // TODO(P02)
    vrfs: z.object({}).passthrough().default({}), // TODO(P02)
    routing: z.object({}).passthrough().default({}), // TODO(P02)
    nat: z.object({}).passthrough().default({}), // TODO(P02)
    objects: z.object({}).passthrough().default({}), // TODO(P02)
    acl: z.object({}).passthrough().default({}), // TODO(P02)
    vpn: z.object({}).passthrough().default({}), // TODO(P02)
    tunnels: z.object({}).passthrough().default({}), // TODO(P02)
    services: z.object({}).passthrough().default({}), // TODO(P02)
    ha: z.object({}).passthrough().default({}), // TODO(P02)
    management: z.object({}).passthrough().default({}), // TODO(P02)
  })
  .strict();

export type RootConfig = z.infer<typeof RootConfig>;

/** Top-level keys in a stable order — used by the diff engine and the UI navigation. */
export const ROOT_KEYS = Object.keys(RootConfig.shape) as (keyof RootConfig)[];
