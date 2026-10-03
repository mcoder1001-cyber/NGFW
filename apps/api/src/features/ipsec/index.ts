import { Ikev2NativeController } from '../ikev2-native/ikev2-native.controller.js';
import { IpsecController } from './ipsec.controller.js';

/** P11's API feature (wave-A-hotspots P1): one import + one spread per array in app.module.ts. */
export const ipsecFeature = {
  controllers: [IpsecController, Ikev2NativeController],
  providers: [],
} as const;
