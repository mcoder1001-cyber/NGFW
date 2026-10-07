"""Read-only independent C2 predicate controls; never execute runner main."""
import copy
import importlib.util
import json
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location('p12_review_source', root / 'test/topology/frr-linuxcp/private-fib.py')
source = importlib.util.module_from_spec(spec)
spec.loader.exec_module(source)
for name in ('gre0', 'gretap0', 'erspan0', 'ip6tnl0'):
    command = ['ip', '-d', '-j', 'address', 'show', 'dev', name]
    rows = json.loads(subprocess.check_output(command))
    assert len(rows) == 1
    link = rows[0]
    assert source.empty_outer_link(link), name
    print(f'{name}: real detailed host fallback accepted')
    for field, value in [('ifname', 'impostor0'), ('flags', ['UP']),
                         ('operstate', 'UNKNOWN'), ('netns-immutable', False),
                         ('link_type', 'veth'), ('master', 'br0'), ('link', 9),
                         ('addr_info', [{'local': '192.0.2.1'}]),
                         ('promiscuity', 1), ('allmulti', 1)]:
        mutant = copy.deepcopy(link)
        mutant[field] = value
        assert not source.empty_outer_link(mutant), (name, field)
    for field in ('remote', 'local', 'ttl'):
        mutant = copy.deepcopy(link)
        mutant['linkinfo']['info_data'][field] = 'configured'
        assert not source.empty_outer_link(mutant), (name, field)
    print(f'{name}: 13 configured/active/impostor negatives rejected')
    accepted_missing = []
    for field in ('flags', 'operstate', 'netns-immutable', 'group', 'promiscuity',
                  'allmulti', 'addr_info', 'link_type', 'linkinfo'):
        mutant = copy.deepcopy(link)
        del mutant[field]
        if source.empty_outer_link(mutant):
            accepted_missing.append(field)
    print(f'{name}: missing required evidence accepted={accepted_missing}')
    assert accepted_missing == ['flags']
print('NO_NAMESPACE_MOUNT_DAEMON_OR_RUNNER_EXECUTION=True')
