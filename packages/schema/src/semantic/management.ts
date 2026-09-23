import { vrfExists } from '../domains/vrfs.js';
import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `management`: the device stays administrable (one enabled admin who can actually log
 * in), usernames are unique, and AAA / syslog VRF references resolve. Role scopes are `'*'` by schema
 * (vdom.md #3), so nothing to check here yet.
 */
export const managementValidators: readonly ValidatorDefinition[] = [
  {
    name: 'management.admin-exists',
    domains: ['management'],
    validate: (config) => {
      const usable = config.management.users.some(
        (u) => u.role === 'admin' && !u.disabled && (u.passwordHash !== undefined || u.sshKeys.length > 0),
      );
      return usable
        ? []
        : [
            {
              pointer: jsonPointer('management', 'users'),
              message: 'at least one enabled admin user with a password or an SSH key is required',
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
    name: 'management.vrf-exists',
    domains: ['management', 'vrfs'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const check = (vrf: string, ...path: (string | number)[]): void => {
        if (!vrfExists(config.vrfs, vrf)) {
          issues.push({ pointer: jsonPointer(...path, 'vrf'), message: `VRF '${vrf}' does not exist` });
        }
      };
      const { aaa, syslog } = config.management;
      for (const [i, s] of aaa.radius.servers.entries()) check(s.vrf, 'management', 'aaa', 'radius', 'servers', i);
      for (const [i, s] of aaa.tacacs.servers.entries()) check(s.vrf, 'management', 'aaa', 'tacacs', 'servers', i);
      for (const [i, s] of syslog.entries()) check(s.vrf, 'management', 'syslog', i);
      return issues;
    },
  },
];
