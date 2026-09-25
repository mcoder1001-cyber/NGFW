/**
 * F-snmp fixtures (kept here, not in `examples/`: `examples.test.ts` assigns every example file to a group by
 * prefix and has no `snmp-` group — see docs/status/tasks/F-snmp-questions.md). Secret refs only, never values.
 */
export const snmpFull = {
  vrfs: {
    default: {
      id: 0,
    },
  },
  interfaces: {
    'TenGigabitEthernet0/0/1': {
      enabled: true,
      ipv4: ['192.168.10.1/24'],
      vrf: 'default',
    },
  },
  services: {
    snmp: {
      enabled: true,
      vrf: 'default',
      listen: [
        {
          address: '192.168.10.1',
        },
      ],
      sysName: 'vrx-a',
      sysLocation: 'rack 12',
      sysContact: 'noc@lab.example',
      sysServices: 72,
      communities: {
        monitoring: {
          secretRef: 'password/snmp-monitoring',
          access: 'ro',
          sources: ['192.168.10.0/24'],
          view: 'mgmt',
        },
      },
      v3Users: {
        noc: {
          securityLevel: 'authPriv',
          authProtocol: 'sha256',
          authRef: 'password/snmp-noc-auth',
          privProtocol: 'aes',
          privRef: 'password/snmp-noc-priv',
        },
      },
      trapReceivers: [
        {
          address: '192.168.10.5',
          version: 'v3',
          user: 'noc',
        },
        {
          address: '192.168.10.6',
          version: 'v2c',
          community: 'monitoring',
        },
      ],
      views: {
        mgmt: {
          include: ['system', 'interfaces', '.1.3.6.1.4.1.99999'],
        },
      },
      monitors: {
        disks: [
          {
            path: '/',
            minPercent: 10,
          },
        ],
        load: {
          max1: 12,
          max5: 10,
          max15: 8,
        },
      },
      subagent: {
        enabled: true,
      },
    },
  },
};

/** snmpFull with a community naming a view that does not exist. */
export const snmpUnknownView = {
  vrfs: {
    default: {
      id: 0,
    },
  },
  interfaces: {
    'TenGigabitEthernet0/0/1': {
      enabled: true,
      ipv4: ['192.168.10.1/24'],
      vrf: 'default',
    },
  },
  services: {
    snmp: {
      enabled: true,
      vrf: 'default',
      listen: [
        {
          address: '192.168.10.1',
        },
      ],
      sysName: 'vrx-a',
      sysLocation: 'rack 12',
      sysContact: 'noc@lab.example',
      sysServices: 72,
      communities: {
        monitoring: {
          secretRef: 'password/snmp-monitoring',
          access: 'ro',
          sources: ['192.168.10.0/24'],
          view: 'nosuch',
        },
      },
      v3Users: {
        noc: {
          securityLevel: 'authPriv',
          authProtocol: 'sha256',
          authRef: 'password/snmp-noc-auth',
          privProtocol: 'aes',
          privRef: 'password/snmp-noc-priv',
        },
      },
      trapReceivers: [
        {
          address: '192.168.10.5',
          version: 'v3',
          user: 'noc',
        },
        {
          address: '192.168.10.6',
          version: 'v2c',
          community: 'monitoring',
        },
      ],
      views: {
        mgmt: {
          include: ['system', 'interfaces', '.1.3.6.1.4.1.99999'],
        },
      },
      monitors: {
        disks: [
          {
            path: '/',
            minPercent: 10,
          },
        ],
        load: {
          max1: 12,
          max5: 10,
          max15: 8,
        },
      },
      subagent: {
        enabled: true,
      },
    },
  },
};
