import { z } from 'zod';

/**
 * DTOs of F-nat46's state routes. NAT46 is stateless (MAP-T 1:1): there are no sessions to page. The routes read the
 * running configuration and compute the RFC 6052 address an IPv4 client appears as on the IPv6 side — what an operator
 * needs to write the IPv6 server's firewall rules and to read its logs.
 */

export const Nat46MappingOut = z.object({
  name: z.string(),
  domain: z.string().describe('VPP map domain name (nat46-<name>)'),
  ipv4: z.string(),
  ipv6: z.string(),
  mtu: z.number().int().nullable(),
});

export const Nat46Out = z.object({
  configured: z.boolean().describe('nat.nat46 is present in the running configuration'),
  clientPrefix: z.string().describe('RFC 6052 /96 IPv4 clients appear under on the IPv6 side'),
  interfaces: z.array(z.string()),
  mappings: z.array(Nat46MappingOut),
});

export const Nat46ClientQuery = z.object({
  ipv4: z.ipv4().describe('an IPv4 client address'),
});
export type Nat46ClientQuery = z.output<typeof Nat46ClientQuery>;

export const Nat46ClientOut = z.object({
  ipv4: z.string(),
  clientPrefix: z.string(),
  ipv6: z.string().describe('the IPv6 source address the IPv6 server sees for this client'),
});
