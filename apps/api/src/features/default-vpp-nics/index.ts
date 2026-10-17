import { SeedService } from './seed.service.js';

/**
 * F-default-vpp-nics (HIST-NATIVE-D-164, D-252): registration of missing data NICs at every API start. No controller — the delete guard
 * lives in `DatastoreService.edit()`, the `/state/interfaces` flags in the existing StateController, and the schema's
 * `dataplane.owner-consistent` rule keeps release/reclaim consistent. `SeedService.start()` is called from main.ts.
 */
export const defaultVppNicsFeature = {
  controllers: [],
  providers: [SeedService],
};

export { SeedService };
