import { SnmpStateController } from './snmp.controller.js';

/** F-snmp API feature (wired in app.module.ts under the F-snmp anchors, P1). */
export const snmpFeature = { controllers: [SnmpStateController], providers: [] };
