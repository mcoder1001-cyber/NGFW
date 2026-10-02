"""Fixed wave-A coverage inventory and fail-closed shared-host boundaries."""
from dataclasses import dataclass
import json
import re
import stat
import time
from pathlib import Path


class Refused(RuntimeError):
    """A required ownership, implementation or evidence boundary is missing."""


@dataclass(frozen=True)
class Stage:
    name: str
    support_module: str
    support_test: str
    required_outcomes: tuple[str, ...]
    gap: str
    # Only reviewed, in-tree composed executors may replace None in a later delta.
    executor: str | None = None


STAGES = (
    Stage('vlan', 'test/topology/vlan-qinq', 'TestVlanQinqTopology',
          ('tagged-ingress', 'forward', 'drop-wrong-tag'), 'counter/ping support; composed tcpdump missing'),
    Stage('bridge-bvi', 'test/topology/bridge-l2', 'TestBridgeL2OnHost',
          ('bridge-forward', 'bvi-route'), 'structural test explicitly has no packets'),
    Stage('vrf-ecmp', 'test/topology/vrf-static-ecmp', 'TestVrfStaticEcmpTopology',
          ('vrf-isolation', 'both-ecmp-paths'), 'FIB/action-ping support; composed multipath packets missing'),
    Stage('urpf-pbr', 'apps/agent', 'TestRpfAdlPbrOnHost',
          ('urpf-valid-forward', 'urpf-spoof-drop', 'pbr-selected-path'), 'subsystem readback support; composed packets missing'),
    Stage('acl', 'test/topology/acl', 'TestACLTopology',
          ('permit', 'deny'), 'separate ping support; composed TCP and capture missing'),
    Stage('nat44-ed', 'test/topology/nat44-ed-sessions', 'TestNat44EdSessions',
          ('translated-source', 'tcp-response'), 'standalone PAT/tcpdump support; composed path missing'),
    Stage('nat44-ei', 'test/topology/nat44-ei-64-66-nptv6', 'TestNatEI6466Nptv6',
          ('translated-source', 'endpoint-independent-mapping', 'tcp-response'), 'standalone PAT/tcpdump support; composed path missing'),
)


def slot_values(slot):
    if isinstance(slot, bool) or not isinstance(slot, int) or slot not in (*range(1, 12), *range(14, 33)):
        raise Refused('developer slot must be 1–11 or 14–32; CI12 and13 are excluded')
    return {
        'VRX_SLOT': str(slot), 'VRX_TEST_PREFIX': f'w{slot}',
        'VRX_HTTP_PORT': str(3000 + 100 * slot if slot <= 11 else 10000 + 100 * slot),
        'VRX_WEB_PORT': str(5000 + 100 * slot if slot <= 11 else 14000 + 100 * slot),
        'VRX_METRICS_PORT': str(9100 + 10 * slot + 1),
        'VRX_AGENT_SOCKET': f'/run/vrx-test/w{slot}/agent.sock',
        'VRX_PG_DATABASE': f'vrx_w{slot}', 'VRX_VALKEY_DB': str(slot),
        'VRX_VPP_TABLE_BASE': str(slot * 1000),
    }


def validate_environment(environment, slot):
    expected = slot_values(slot)
    if environment.get('VRX_INTEGRATION') != '1' or environment.get('VRX_TRAFFIC_A_HOST') != '1':
        raise Refused('explicit VRX_INTEGRATION=1 and VRX_TRAFFIC_A_HOST=1 required')
    for name, value in expected.items():
        if environment.get(name) != value:
            raise Refused(f'{name} differs from the allocated slot')
    if environment.get('VRX_GLOBALS_OWNER', '0') != '0' or environment.get('VRX_VPP_ID_RANGE'):
        raise Refused('traffic worker cannot own global VPP settings or all IDs')
    forbidden = ('VRX_ACL_STATS_GLOBALS', 'VRX_DF7_GLOBALS', 'VRX_DF8_GLOBALS', 'VRX_NAT64_TENANT_VRF_HOST')
    if any(environment.get(name) not in (None, '', '0') for name in forbidden):
        raise Refused('unsupported global/retained-state integration option')
    return expected


def validate_lease(path, slot, boot_id, now=None):
    """Read a manager-provisioned lease; never create or renew one ourselves."""
    slot_values(slot)
    expected = f'/run/vrx-test/w{slot}/traffic-a-lease.json'
    if str(path) != expected:
        raise Refused('lease must be the fixed allocated-slot path')
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) != 0o600 or not 0 < info.st_size <= 4096:
        raise Refused('lease must be a bounded root-owned regular0600 file')
    data = json.loads(path.read_text())
    if set(data) != {'task', 'slot', 'prefix', 'boot_id', 'expires_unix', 'lease_id'}:
        raise Refused('lease fields differ from the explicit manager contract')
    if (data['task'] != 'TEST-traffic-A' or type(data['slot']) is not int or data['slot'] != slot
            or data['prefix'] != f'w{slot}' or data['boot_id'] != boot_id
            or not isinstance(data['lease_id'], str) or not re.fullmatch(r'[0-9a-f]{32}', data['lease_id'])):
        raise Refused('lease does not identify this task, boot and slot')
    current = time.time() if now is None else now
    if type(data['expires_unix']) not in (int, float) or not current < data['expires_unix'] <= current + 7200:
        raise Refused('lease expired or exceeds the bounded two-hour window')
    return data


def require_implemented(stages=STAGES):
    missing = [stage.name for stage in stages if stage.executor is None]
    if not stages or missing:
        raise Refused('NOTIMPLEMENTED: composed executors/capture contracts missing for ' + ', '.join(missing))


def plan(slot):
    values = slot_values(slot)
    prefix = values['VRX_TEST_PREFIX']
    return {'task': 'TEST-traffic-A', 'status': 'NOTIMPLEMENTED', 'slot': slot,
            'path': 'af_packet', 'whole_chain_proven': False,
            'captures': [{'side': side, 'namespace': f'ns-{prefix}-{side}',
                          'device': f'{prefix}{letter}1'} for side, letter in (('lan', 'l'), ('wan', 'w'))],
            'stages': [{'name': stage.name, 'status': 'NOTIMPLEMENTED',
                        'support_module': stage.support_module, 'support_test': stage.support_test,
                        'required_outcomes': stage.required_outcomes, 'gap': stage.gap} for stage in STAGES]}
