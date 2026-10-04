import { SeedService } from './seed.service.js';

/**
 * F-default-vpp-nics (D-164): first-boot seeding of the default dataplane document. No controller — the delete guard
 * lives in `DatastoreService.edit()`, the `/state/interfaces` flags in the existing StateController, and the schema's
 * `dataplane.owner-consistent` rule keeps release/reclaim consistent. `SeedService.start()` is called from main.ts.
 */
export const defaultVppNicsFeature = {
  controllers: [],
  providers: [SeedService],
};

export { SeedService };
