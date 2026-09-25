import { LispController } from './lisp.controller.js';

/**
 * F-lisp API feature: `GET /api/v1/state/lisp` (agent LispState). The configuration lives at `tunnels.lisp` and is
 * edited through the generic pointer routes (`/api/v1/config/tunnels/lisp…`); nothing LISP-specific is needed there.
 */
export const lispFeature = {
  controllers: [LispController],
  providers: [],
} as const;
