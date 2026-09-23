import { vrfExists } from '../domains/vrfs.js';
import { canonicalIp } from '../ip.js';
import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { duplicateIssues } from './unique.js';

/** Host key for uniqueness: canonical IP, or the lower-cased host name. */
const hostKey = (address: string): string => canonicalIp(address) ?? address.toLowerCase();

/**
 * Semantic validators for `management`: once local users are configured the device stays administrable (one
 * enabled admin who can actually log in — an empty list is valid, the API seeds the first admin, D-048),
 * usernames and AAA / syslog servers are unique, and AAA / syslog VRF references resolve. Role scopes are `'*'` by
 * schema (vdom.md #3), so nothing to check here yet.
 */
export const managementValidators: readonly ValidatorDefinition[] = [
  {
    name: 'management.admin-exists',
    domains: ['management'],
    validate: (config) => {
      const users = config.management.users;
      if (users.length === 0) return []; // D-048: bootstrap is the API's job (it seeds the first admin)
      const usable = users.some(
        (u) =>
          u.role === 'admin' &&
          !u.disabled &&
          (u.passwordHash !== undefined || u.sshKeys.length > 0),
      );
      return usable
        ? []
        : [
            {
              pointer: jsonPointer('management', 'users'),
              message:
                'at least one enabled admin user with a password or an SSH key is required (removing, disabling or demoting the last one would lock everybody out)',
            },
          ];
    },
  },
  {
    name: 'management.username-unique',
    domains: ['management'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const seen = new Set<string>();
      for (const [i, user] of config.management.users.entries()) {
        if (seen.has(user.username)) {
          issues.push({
            pointer: jsonPointer('management', 'users', i, 'username'),
            message: `username '${user.username}' is already taken`,
          });
        }
        seen.add(user.username);
      }
      return issues;
    },
  },
  {
    name: 'management.server-unique',
    domains: ['management'],
    validate: (config) => {
      const { aaa, syslog } = config.management;
      return [
        ...duplicateIssues(
          aaa.radius.servers,
          (s) => `${hostKey(s.address)} ${s.authPort}`,
          (_s, i) => ['management', 'aaa', 'radius', 'servers', i, 'address'],
          (s) => `RADIUS server ${s.address} port ${s.authPort} is listed more than once`,
        ),
        ...duplicateIssues(
          aaa.tacacs.servers,
          (s) => `${hostKey(s.address)} ${s.port}`,
          (_s, i) => ['management', 'aaa', 'tacacs', 'servers', i, 'address'],
          (s) => `TACACS+ server ${s.address} port ${s.port} is listed more than once`,
        ),
        ...duplicateIssues(
          syslog,
          (s) => `${hostKey(s.address)} ${s.port} ${s.protocol}`,
          (_s, i) => ['management', 'syslog', i, 'address'],
          (s) => `syslog collector ${s.protocol}://${s.address}:${s.port} is listed more than once`,
        ),
      ];
    },
  },
  {
    name: 'management.vrf-exists',
    domains: ['management', 'vrfs'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const check = (vrf: string, ...path: (string | number)[]): void => {
        if (!vrfExists(config.vrfs, vrf)) {
          issues.push({
            pointer: jsonPointer(...path, 'vrf'),
            message: `VRF '${vrf}' does not exist`,
          });
        }
      };
      const { aaa, syslog } = config.management;
      for (const [i, s] of aaa.radius.servers.entries())
        check(s.vrf, 'management', 'aaa', 'radius', 'servers', i);
      for (const [i, s] of aaa.tacacs.servers.entries())
        check(s.vrf, 'management', 'aaa', 'tacacs', 'servers', i);
      for (const [i, s] of syslog.entries()) check(s.vrf, 'management', 'syslog', i);
      return issues;
    },
  },
];
