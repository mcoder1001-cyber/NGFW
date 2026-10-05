import { z } from 'zod';
import { withUi } from '../../ui.js';

const address = z.ipv4().refine((a) => {
  const first = Number(a.split('.')[0]);
  return first > 0 && first < 224 && first !== 127;
}, 'HA endpoint must be a unicast IPv4 address');

export const HaNatListenerSchema = z.strictObject({
  address: withUi(address, { title: 'NAT HA listener IPv4 address' }),
  port: withUi(z.int().min(1).max(65535).default(8750), { title: 'NAT HA UDP port' }),
  pathMtu: withUi(z.int().min(576).max(9000).default(1500), { title: 'HA path MTU' }),
});
export const HaNatFailoverSchema = z.strictObject({
  address: withUi(address, { title: 'NAT HA peer IPv4 address' }),
  port: withUi(z.int().min(1).max(65535).default(8750), { title: 'NAT HA peer UDP port' }),
  sessionRefreshSec: withUi(z.int().min(1).max(3600).default(10), {
    title: 'HA session refresh seconds',
  }),
});
