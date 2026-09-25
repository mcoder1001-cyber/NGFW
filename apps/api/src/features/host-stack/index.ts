import { HostStackController } from './host-stack.controller.js';

/** F-host-stack feature module parts (wired in app.module.ts under the feature's anchor, wave-A-hotspots P1). */
export const hostStackFeature = {
  controllers: [HostStackController],
  providers: [],
} as const;

export { hostStackFake } from './fake.js';
