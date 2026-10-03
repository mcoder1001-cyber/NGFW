#!/usr/bin/env python3
"""Run independent test modules without hiding later suites after a failure.

Topology suites use a fresh disposable VPP each. Results and raw logs are retained.
"""
import json
import os
from pathlib import Path
import shlex
import signal
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / 'docs/status/tasks/full-test-2026-10-03-evidence'
HEAVY = str(ROOT / 'tools/heavy.sh')
ISOLATED = str(Path(__file__).with_name('isolated-vpp.py'))


def main():
    group = sys.argv[1]
    OUT.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ)
    env.pop('VRX_INTEGRATION', None)
    for line in subprocess.check_output([ROOT / 'tools/lab', 'env', os.environ.get('VRX_TEST_SLOT', '10')], text=True).splitlines():
        if line.startswith('export '):
            key, value = line[7:].split('=', 1)
            env[key] = shlex.split(value)[0]
    agent = str(ROOT / 'apps/agent/bin/vrx-agent')
    ctl = str(ROOT / 'apps/agent/bin/vrx-agentctl')
    preflight = str(ROOT / 'apps/agent/bin/vrx-vpp-preflight')
    for key in ['VRX_BL2_AGENT_BIN', 'VRX_OM_AGENT_BIN', 'VRX_ACL_AGENT_BIN', 'VRX_VSE_AGENT_BIN',
                'VRX_LBGS_AGENT_BIN', 'VRX_P08_AGENT_BIN', 'VRX_QINQ_AGENT_BIN', 'VRX_NAT_AGENT_BIN',
                'VRX_HA_AGENT_BIN', 'VRX_UCS_AGENT_BIN', 'VRX_NRA_AGENT_BIN', 'VRX_F_BONDING_AGENT_BIN',
                'VRX_KEA_AGENT_BIN', 'VRX_SYSID_AGENT_BIN', 'VRX_QOS_AGENT_BIN']:
        env[key] = agent
    env.update(VRX_OM_AGENTCTL_BIN=ctl, VRX_ACL_AGENTCTL_BIN=ctl, VRX_HA_AGENTCTL_BIN=ctl,
               VRX_ACL_PREFLIGHT_BIN=preflight, VRX_PREFLIGHT_BIN=preflight)
    results = []
    def run(label, command, timeout=1200, extra=None, isolated=False):
        started = time.time()
        child_env = dict(env)
        if extra: child_env.update(extra)
        argv = [HEAVY]
        if isolated: argv += [sys.executable, ISOLATED]
        argv += list(map(str, command))
        print('START', group, label, flush=True)
        with (OUT / (group + '-' + label + '.log')).open('w') as log:
            log.write('$ ' + shlex.join(argv) + '\n'); log.flush()
            process = subprocess.Popen(argv, cwd=ROOT, env=child_env, stdout=log,
                                       stderr=subprocess.STDOUT, start_new_session=True)
            try:
                rc = process.wait(timeout=timeout)
            except subprocess.TimeoutExpired:
                # Stop only the process group created for this finite test.
                os.killpg(process.pid, signal.SIGTERM)
                try: process.wait(timeout=20)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.wait()
                rc = 124
        counts = {'pass': 0, 'fail': 0, 'skip': 0}
        for line in (OUT / (group + '-' + label + '.log')).read_text().splitlines():
            try: event = json.loads(line)
            except (json.JSONDecodeError, ValueError): continue
            if event.get('Test') and event.get('Action') in counts:
                counts[event['Action']] += 1
        results.append(dict(label=label, command=command, exit=rc, elapsed=round(time.time()-started, 2), tests=counts))
        (OUT / (group + '-manifest.json')).write_text(json.dumps(results, indent=2))
        print('END', group, label, 'exit', rc, flush=True)

    if group == 'unit-modules':
        for module in sorted((ROOT / 'test').rglob('go.mod')):
            relative = module.parent.relative_to(ROOT)
            run(str(relative).replace('/', '_'), ['go', '-C', str(relative), 'test', '-json', '-count=1', '-race', '-p', '1', '-timeout', '8m', './...'])
        run('terraform', ['go', '-C', 'sdk/terraform', 'test', '-json', '-race', '-count=1', '-timeout', '8m', './...'])
        run('vpp-build-guards', ['bash', 'deploy/vpp/tests/run.sh'])
    elif group == 'topology':
        if not Path(agent).is_file(): raise SystemExit('build agent binaries first')
        if not (ROOT / 'apps/api/dist/main.js').is_file(): raise SystemExit('build API first')
        for module in sorted((ROOT / 'test').rglob('go.mod')):
            relative = module.parent.relative_to(ROOT)
            extra = {'VRX_INTEGRATION': '1'}
            if relative.name == 'det44': extra['VRX_FDET44_DET44_HOST'] = '1'
            if relative.name == 'acl': extra['VRX_ACL_EDITOR'] = '1'
            if relative.name == 'ipfix-sflow': extra['VRX_IPFIX_GLOBALS'] = '1'
            run(str(relative).replace('/', '_'), ['go', '-C', str(relative), 'test', '-json', '-count=1', '-race', '-p', '1', '-timeout', '12m', './...'],
                extra=extra, isolated=True, timeout=1500)
    elif group == 'opt-in':
        cases = [
            ('wireguard', './internal/agent', '^TestWireguardHandshakeOnHost$', {'VRX_WG_HANDSHAKE':'1'}),
            ('srv6-globals', './internal/agent', '^TestSrv6GlobalsOnHost$', {'VRX_FSRV6_GLOBALS':'1'}),
            ('mpls-globals', './internal/agent', '^TestMplsOnHost$', {'VRX_DF7_GLOBALS':'1'}),
            ('lb', './internal/agent', '^TestLb(OnHost|GarbageCollectOnHost)$', {'VRX_LB_HOST':'1','VRX_LB_GLOBALS':'1'}),
            ('ospf-fib', './internal/agent', 'OSPF.*Topology|Ospf.*Topology', {'VRX_OSPF_TOPOLOGY':'1','VRX_OSPF_FIB':'root'}),
            ('bgp-fib', './internal/agent', 'P12.*Topology', {'VRX_P12_TOPOLOGY':'1','VRX_P12_FIB':'root'}),
            ('igmp', './internal/descriptors/igmp', 'OnHost', {'VRX_DF7_IGMP_HOST':'1'}),
            ('vrrp', './internal/descriptors/vrrp', 'OnHost', {'VRX_DF7_VRRP_HOST':'1'}),
            ('dns', './internal/descriptors/dns', 'OnHost', {'VRX_DNS_VPP_HOST':'1','VRX_DF8_GLOBALS':'1'}),
            ('gtpu', './internal/descriptors/gtpu', 'Host', {'VRX_DF6_GTPU_HOST':'1'}),
            ('l2tp', './internal/descriptors/l2tp', 'Host', {'VRX_DF6_L2TP_CREATE':'1'}),
            ('lisp', './internal/descriptors/lisp', 'Host', {'VRX_DF6_LISP_HOST':'1'}),
            ('det44', './internal/descriptors/det44', 'Host', {'VRX_DF3_DET44':'1'}),
            ('pppoe-cp', './internal/descriptors/pppoe', 'Host', {'VRX_DF6_PPPOE_CP_HOST':'1'}),
            ('globals-df5', './internal/descriptors/ikev2', 'Host', {'VRX_DF5_GLOBALS':'1'}),
            ('globals-dhcp', './internal/descriptors/dhcp', 'Host', {'VRX_DF8_GLOBALS':'1','VRX_DF8_DUID':'1'}),
            ('globals-lcp', './internal/descriptors/lcp', 'Host', {'VRX_DF8_GLOBALS':'1','VRX_DF8_LCP_REPLACE':'1'}),
            ('globals-ipfix', './internal/descriptors/ipfix', 'Host', {'VRX_DF8_GLOBALS':'1'}),
            ('globals-sflow', './internal/descriptors/sflow', 'Host', {'VRX_DF8_GLOBALS':'1'}),
            ('globals-flowprobe', './internal/descriptors/flowprobe', 'Host', {'VRX_DF8_GLOBALS':'1'}),
            ('globals-pcap', './internal/descriptors/pcap', 'Host', {'VRX_DF8_GLOBALS':'1'}),
            ('proxy-nd', './internal/descriptors/ip6_nd', 'Host', {'VRX_DF2_PROXY_ND':'1'}),
            ('nsim', './internal/descriptors/nsim', 'Host', {'VRX_NSIM_HOST':'1'}),
            ('auto-sdl', './internal/descriptors/auto_sdl', 'Host', {'VRX_AUTOSDL_GLOBALS':'1'}),
            ('strongswan-stock', './internal/renderers/strongswan', 'Integration', {}),
        ]
        for label, package, pattern, extra in cases:
            extra['VRX_INTEGRATION'] = '1'
            run(label, ['go', '-C', 'apps/agent', 'test', '-json', '-race', '-count=1', '-timeout', '6m', '-run', pattern, package], extra=extra, isolated=True)
    elif group == 'topology-retry':
        wrapper = str(ROOT / 'test/topology/hardware-smoke/agent-isolated-wrapper.sh')
        for key in list(env):
            if key.endswith('_AGENT_BIN'): env[key] = wrapper
        for name in ['bridge-l2', 'host-acl-nftables', 'loopback-bvi-gso-lldp-span', 'nat44-ed-sessions', 'nat44-ei-64-66-nptv6', 'object-model', 'vlan-qinq', 'bonding', 'neighbors-ra', 'kea-dhcp-relay']:
            run(name, ['go', '-C', 'test/topology/'+name, 'test', '-json', '-race', '-p', '1', '-count=1', '-timeout', '10m', './...'], extra={'VRX_INTEGRATION':'1'}, isolated=True)
    elif group == 'acl-extra':
        extra = {'VRX_INTEGRATION':'1','VRX_ACL_JANITOR':'1','VRX_ACL_STATS_GLOBALS':'1','VRX_ACL_SCALE':'100000'}
        run('acl-scale-janitor', ['go', '-C', 'test/topology/acl', 'test', '-json', '-race', '-count=1', '-timeout', '15m', '-run', 'TestACLTopology|TestACLJanitor', './...'], extra=extra, isolated=True, timeout=1500)
    elif group == 'extra':
        cases = [
            ('bfd', './internal/descriptors/bfd', 'Host', {'VRX_DF7_GLOBALS':'1'}),
            ('lldp', './internal/descriptors/lldp', 'Host', {'VRX_DF7_GLOBALS':'1'}),
            ('mpls', './internal/descriptors/mpls', 'Host', {'VRX_DF7_GLOBALS':'1'}),
            ('lb-descriptor', './internal/descriptors/lb', 'Host', {'VRX_DF7_LB':'1'}),
            ('trace', './internal/descriptors/trace', 'Host', {'VRX_DF8_GLOBALS':'1'}),
            ('workers-policer', './internal/descriptors/policer', 'Host', {'VRX_ISOLATED_WORKERS':'2'}),
            ('workers-interface', './internal/descriptors/interface', 'Host', {'VRX_ISOLATED_WORKERS':'2'}),
            ('objects-scale', './internal/objects', 'TestStoreScale', {'VRX_OBJECTS_SCALE':'1000,2000,4000'}),
            ('scheduler-perf', './internal/scheduler', 'TestTD21Scale', {'VRX_PERF':'1'}),
        ]
        for label, package, pattern, extra in cases:
            extra['VRX_INTEGRATION'] = '1'
            run(label, ['go', '-C', 'apps/agent', 'test', '-json', '-race', '-count=1', '-timeout', '10m', '-run', pattern, package], extra=extra, isolated=True)
    else:
        raise SystemExit('group: unit-modules | topology | opt-in')
    raise SystemExit(1 if any(result['exit'] for result in results) else 0)


if __name__ == '__main__':
    main()
