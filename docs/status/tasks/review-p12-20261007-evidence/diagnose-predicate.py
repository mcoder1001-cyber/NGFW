#!/usr/bin/env python3
"""Read-only host inventory and exact source-predicate reproduction; no runner."""
import ast
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[4]
source = ROOT / 'test/topology/frr-linuxcp/private-fib.py'
tree = ast.parse(source.read_text())
matches = []
for node in ast.walk(tree):
    if isinstance(node, ast.If) and any(
        isinstance(child, ast.Constant)
        and child.value == 'P12 outer network namespace is not empty'
        for child in ast.walk(node)
    ):
        matches.append(node)
assert len(matches) == 1
condition = matches[0].test
print('EXACT_SOURCE_PREDICATE=' + ast.unparse(condition))
expression = compile(ast.Expression(condition), str(source), 'eval')
host = json.loads(subprocess.check_output(['ip', '-d', '-j', 'link', 'show'], text=True))
fallback = {link['ifname']: link for link in host
            if link['ifname'] in {'gre0', 'gretap0', 'erspan0'}}
assert set(fallback) == {'gre0', 'gretap0', 'erspan0'}
for link in fallback.values():
    assert link['operstate'] == 'DOWN'
    assert 'UP' not in link['flags']
    assert link['netns-immutable'] is True
    print('HOST_DEFAULT_LINK=' + json.dumps(link, sort_keys=True))
base = [{'ifname': 'lo'}, {'ifname': 'ip6tnl0'}]
for repeat in (1, 2):
    assert eval(expression, {'any': any}, {'links': base}) is False
    print('REPEAT_' + str(repeat) + '_LO_IP6TNL_ALLOWED=True')
    for name, link in fallback.items():
        rejected = eval(expression, {'any': any}, {'links': base + [link]})
        assert rejected is True
        print('REPEAT_' + str(repeat) + '_KERNEL_DEFAULT_' + name + '_REJECTED=True')
print('NO_NETNS_MOUNTS_DAEMONS_OR_RUNNER_EXECUTED=True')
