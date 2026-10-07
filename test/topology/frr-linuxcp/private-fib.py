#!/usr/bin/env python3
"""P12 root-mode acceptance inside an owned outer netns and disposable VPP."""
import fcntl
import errno
from contextlib import ExitStack
import json
import os
from pathlib import Path
import subprocess
import sys
import signal
import time

ROOT = Path(__file__).resolve().parents[3]
SELF = str(Path(__file__).resolve())


def empty_outer_link(link):
    """Accept loopback or strictly unconfigured immutable kernel fallbacks."""
    if link.get('ifname') == 'lo':
        return link.get('link_type') == 'loopback' and 'LOOPBACK' in link.get('flags', [])
    defaults = {
        'gre0': ('gre', 'gre', {'remote': 'any', 'local': 'any', 'ttl': 0, 'pmtudisc': False}),
        'gretap0': ('gretap', 'ether', {'remote': 'any', 'local': 'any', 'ttl': 0, 'pmtudisc': False}),
        'erspan0': ('erspan', 'ether', {'remote': 'any', 'local': 'any', 'ttl': 0,
                                      'pmtudisc': False, 'okey': '0.0.0.0',
                                      'erspan_index': 0, 'erspan_ver': 1}),
        'ip6tnl0': ('ip6tnl', 'tunnel6', {'proto': 'ip6ip6', 'remote': 'any', 'local': 'any',
                                        'ttl': 0, 'encap_limit': 0, 'tclass': '0x00',
                                        'flowlabel': '0x00000'}),
    }
    expected = defaults.get(link.get('ifname'))
    if expected is None:
        return False
    kind, link_type, data = expected
    info = link.get('linkinfo', {})
    return (link.get('netns-immutable') is True and link.get('operstate') == 'DOWN'
            and not {'UP', 'LOWER_UP', 'MASTER'}.intersection(link.get('flags', []))
            and link.get('link') is None and 'master' not in link
            and link.get('group') == 'default' and link.get('promiscuity') == 0
            and link.get('allmulti') == 0 and link.get('addr_info') == []
            and link.get('link_type') == link_type and info.get('info_kind') == kind
            and info.get('info_data') == data)


def snapshot():
    commands = [
        ['systemctl', 'show', 'vpp', '-p', 'MainPID', '-p', 'NRestarts'],
        ['ip', '-4', 'route', 'show'],
        ['vppctl', '-s', '/run/vpp/cli.sock', 'show', 'lcp'],
    ]
    return [subprocess.check_output(command).decode() for command in commands]


def process_identity(pid):
    try:
        stat = Path(f'/proc/{pid}/stat').read_text()
        return (stat[stat.rindex(')') + 1:].split()[19], os.readlink(f'/proc/{pid}/ns/net'))
    except (FileNotFoundError, ProcessLookupError):
        return None


def members(namespaces):
    found = {}
    for entry in Path('/proc').iterdir():
        if entry.name.isdigit() and int(entry.name) != os.getpid():
            identity = process_identity(entry.name)
            if identity and identity[1] in namespaces:
                found[int(entry.name)] = identity
    return found


def retain_namespaces(current, retained, handles, observed):
    # The outer mount sees placeholder files. Inspect /run/netns through each
    # actual outer-netns process's root, including the inner mount launcher.
    # NS_GET_NSTYPE validates nsfs, and open handles prevent inode reuse after
    # peer handles are unmounted. A regular placeholder never proves ownership.
    processes = members({current})
    processes[os.getpid()] = process_identity(os.getpid())
    forbidden = {os.environ['NGFW_P12_HOST_NETNS'], os.readlink('/proc/1/ns/net')}
    for pid, identity in processes.items():
        directory = Path(f'/proc/{pid}/root/run/netns')
        try:
            entries = list(directory.iterdir())
        except FileNotFoundError:
            if process_identity(pid) != identity:
                continue
            raise
        for handle in entries:
            if handle.name not in {'ns-w14-lan', 'ns-w14-wan', 'ns-w14-frr'}:
                raise RuntimeError('unexpected P12 namespace handle: ' + handle.name)
            try:
                fd = os.open(handle, os.O_RDONLY | os.O_CLOEXEC)
            except FileNotFoundError:
                continue  # handle concurrently removed; retained fds survive
            try:
                try:
                    kind = fcntl.ioctl(fd, 0xb703)  # NS_GET_NSTYPE
                except OSError as error:
                    if error.errno == errno.ENOTTY:
                        continue  # outer mount's ordinary placeholder file
                    raise
                if kind != 0x40000000:  # CLONE_NEWNET
                    raise RuntimeError('P12 handle is not a network namespace')
                namespace = 'net:[' + str(os.fstat(fd).st_ino) + ']'
                if namespace in forbidden or namespace == current:
                    raise RuntimeError('P12 peer handle points to shared host')
                observed.add(handle.name)
                if namespace not in retained:
                    retained[namespace] = fd
                    handles.callback(os.close, fd)
                    fd = None
            finally:
                if fd is not None:
                    os.close(fd)


def private_run(command, runtime, current):
    with ExitStack() as handles:
        return tracked_private_run(command, current, handles)


def tracked_private_run(command, current, handles):
    # Track actual private namespaces while named peers exist; retain their inode
    # identity after handles disappear so surviving daemons cannot escape cleanup.
    namespaces = {current}
    retained = {}
    observed = set()
    process = subprocess.Popen(command, cwd=ROOT)
    result = 1
    try:
        deadline = time.monotonic() + 1250
        while process.poll() is None:
            retain_namespaces(current, retained, handles, observed)
            namespaces.update(retained)
            if time.monotonic() >= deadline:
                raise RuntimeError('P12 private runner deadline exceeded')
            time.sleep(.1)
        result = process.returncode
    finally:
        # Include the last observed handles even when the command just exited.
        try:
            retain_namespaces(current, retained, handles, observed)
        except (OSError, RuntimeError) as error:
            print('P12_NAMESPACE_INVENTORY_ERROR=' + str(error), flush=True)
            result = result or 1
        namespaces.update(retained)
        print('P12_RETAINED_NAMESPACES=' + json.dumps(sorted(retained)), flush=True)
        leaked = members(namespaces)
        print('P12_PRIVATE_REMAINING=' + json.dumps(leaked), flush=True)
        # Each signal targets one observed PID, rechecking immutable starttime and
        # membership. The host namespace can never enter this set.
        for sig in [signal.SIGTERM, signal.SIGKILL]:
            for pid, identity in leaked.items():
                if (identity[1] != os.environ['NGFW_P12_HOST_NETNS']
                        and process_identity(pid) == identity):
                    try:
                        os.kill(pid, sig)
                    except ProcessLookupError:
                        pass
            until = time.monotonic() + 5
            while members(namespaces) and time.monotonic() < until:
                time.sleep(.1)
            if not members(namespaces):
                break
        process.wait(timeout=10)
        remaining = members(namespaces)
        print('P12_PRIVATE_PROCESSES_AFTER=' + json.dumps(remaining), flush=True)
        if leaked or remaining:
            result = result or 1
        # Success requires observing both peer scopes, even if already empty.
        if result == 0 and not {'ns-w14-lan', 'ns-w14-wan'}.issubset(observed):
            print('P12 peer namespace retention incomplete', flush=True)
            result = 1
    return result


def self_test():
    """Host-independent controls; no namespaces, daemons or mounts are created."""
    import unittest
    from types import SimpleNamespace
    from unittest.mock import MagicMock, patch

    class Controls(unittest.TestCase):
        def fallback(self):
            return {'ifname': 'gre0', 'link_type': 'gre', 'netns-immutable': True,
                    'operstate': 'DOWN', 'flags': ['NOARP'], 'link': None,
                    'group': 'default', 'promiscuity': 0, 'allmulti': 0, 'addr_info': [],
                    'linkinfo': {'info_kind': 'gre', 'info_data': {
                        'remote': 'any', 'local': 'any', 'ttl': 0, 'pmtudisc': False}}}

        def test_unconfigured_kernel_fallbacks_accepted(self):
            link = self.fallback()
            self.assertTrue(empty_outer_link(link))
            for name in ['gretap0', 'erspan0']:
                link['ifname'] = name
                link['link_type'] = 'ether'
                link['linkinfo']['info_kind'] = name[:-1]
                if name == 'erspan0':
                    link['linkinfo']['info_data'].update(okey='0.0.0.0', erspan_index=0, erspan_ver=1)
                self.assertTrue(empty_outer_link(link), name)
            self.assertTrue(empty_outer_link({'ifname': 'lo', 'link_type': 'loopback',
                                             'flags': ['LOOPBACK', 'UP']}))
            link.update(ifname='ip6tnl0', link_type='tunnel6')
            link['linkinfo'] = {'info_kind': 'ip6tnl', 'info_data': {
                'proto': 'ip6ip6', 'remote': 'any', 'local': 'any', 'ttl': 0,
                'encap_limit': 0, 'tclass': '0x00', 'flowlabel': '0x00000'}}
            self.assertTrue(empty_outer_link(link))

        def test_active_configured_or_impostor_defaults_rejected(self):
            import copy
            mutations = [
                {'flags': ['NOARP', 'UP']}, {'flags': ['LOWER_UP']}, {'operstate': 'UNKNOWN'},
                {'netns-immutable': False}, {'link_type': 'ether'}, {'link': 9}, {'master': 'br0'},
                {'group': 'other'}, {'promiscuity': 1}, {'allmulti': 1},
                {'addr_info': [{'local': '192.0.2.1'}]}, {'ifname': 'eth0'},
                {'linkinfo': {'info_kind': 'veth', 'info_data': {}}},
                {'linkinfo': {'info_kind': 'gre', 'info_data': {'remote': '192.0.2.1'}}},
            ]
            for mutation in mutations:
                with self.subTest(mutation=mutation):
                    link = copy.deepcopy(self.fallback())
                    link.update(mutation)
                    self.assertFalse(empty_outer_link(link))
            for field in ['addr_info', 'netns-immutable', 'linkinfo']:
                link = self.fallback()
                del link[field]
                self.assertFalse(empty_outer_link(link))
            link = self.fallback()
            link['linkinfo']['info_data']['ikey'] = '0.0.0.1'
            self.assertFalse(empty_outer_link(link))
            self.assertFalse(empty_outer_link({'ifname': 'lo', 'link_type': 'veth', 'flags': []}))

        def exercise(self, kind=0x40000000, inode=777, repeat=False):
            directory = MagicMock()
            directory.iterdir.return_value = [SimpleNamespace(name='ns-w14-lan')]
            retained, observed = {}, set()
            with ExitStack() as mocks:
                mocks.enter_context(patch(__name__ + '.members', return_value={123: ('start', 'private')}))
                mocks.enter_context(patch(__name__ + '.process_identity', return_value=('start', 'private')))
                mocks.enter_context(patch(__name__ + '.Path', return_value=directory))
                mocks.enter_context(patch.dict(os.environ, NGFW_P12_HOST_NETNS='net:[1]'))
                mocks.enter_context(patch.object(os, 'readlink', return_value='net:[1]'))
                mocks.enter_context(patch.object(os, 'open', return_value=42))
                mocks.enter_context(patch.object(os, 'fstat', return_value=SimpleNamespace(st_ino=inode)))
                mocks.enter_context(patch.object(fcntl, 'ioctl', return_value=kind,
                                                 side_effect=kind if isinstance(kind, Exception) else None))
                close = mocks.enter_context(patch.object(os, 'close'))
                with ExitStack() as handles:
                    retain_namespaces('private', retained, handles, observed)
                    if repeat:
                        retain_namespaces('private', retained, handles, observed)
                self.assertGreaterEqual(close.call_count, 1)
            return retained, observed

        def test_actual_nsfs_retained_and_deduplicated(self):
            retained, observed = self.exercise(repeat=True)
            self.assertEqual(list(retained), ['net:[777]'])
            self.assertEqual(observed, {'ns-w14-lan'})

        def test_placeholder_never_counts(self):
            self.assertEqual(self.exercise(OSError(errno.ENOTTY, 'placeholder')), ({}, set()))

        def test_host_handle_rejected(self):
            with self.assertRaisesRegex(RuntimeError, 'shared host'):
                self.exercise(inode=1)

        def test_wrong_namespace_type_rejected(self):
            with self.assertRaisesRegex(RuntimeError, 'network namespace'):
                self.exercise(kind=0x20000)

        def test_inventory_permission_error_not_empty_success(self):
            with patch(__name__ + '.members', side_effect=PermissionError('inventory denied')):
                with ExitStack() as handles, self.assertRaises(PermissionError):
                    retain_namespaces('private', {}, handles, set())

        def test_deleted_handle_retention_and_orphan_failure(self):
            # Model actual retained peer scopes after their named handles vanish.
            # An orphan makes acceptance fail even when exact-PID cleanup succeeds.
            retained = {'net:[777]': 42, 'net:[778]': 43}
            observed = {'ns-w14-lan', 'ns-w14-wan'}
            orphan = {456: ('start', 'net:[777]')}
            process = MagicMock()
            process.poll.return_value = 0
            process.returncode = 0
            def retain(current, target, handles, names):
                target.update(retained)
                names.update(observed)
            with patch(__name__ + '.retain_namespaces', side_effect=retain), \
                    patch(__name__ + '.members', side_effect=[orphan, {}, {}, {}]) as inventory, \
                    patch(__name__ + '.process_identity', return_value=('start', 'net:[777]')), \
                    patch.dict(os.environ, NGFW_P12_HOST_NETNS='net:[1]'), \
                    patch.object(subprocess, 'Popen', return_value=process), \
                    patch.object(os, 'kill') as kill:
                self.assertEqual(private_run(['unused'], None, 'private'), 1)
                self.assertIn('net:[777]', inventory.call_args_list[0].args[0])
                kill.assert_called_once_with(456, signal.SIGTERM)

        def test_persistent_inventory_failure_closes_retained_descriptors(self):
            process = MagicMock()
            process.poll.return_value = 0
            process.returncode = 0
            def retain(current, target, handles, names):
                target['net:[777]'] = 42
                handles.callback(os.close, 42)
            with patch(__name__ + '.retain_namespaces', side_effect=retain), \
                    patch(__name__ + '.members', side_effect=PermissionError('inventory denied')), \
                    patch.object(subprocess, 'Popen', return_value=process), \
                    patch.object(os, 'close') as close, \
                    patch.object(os, 'kill') as kill:
                with self.assertRaises(PermissionError):
                    private_run(['unused'], None, 'private')
                close.assert_called_once_with(42)
                kill.assert_not_called()

        def test_success_without_observed_peer_scopes_rejected(self):
            process = MagicMock()
            process.poll.return_value = 0
            process.returncode = 0
            with patch(__name__ + '.retain_namespaces'), \
                    patch(__name__ + '.members', return_value={}), \
                    patch.object(subprocess, 'Popen', return_value=process):
                self.assertEqual(private_run(['unused'], None, 'private'), 1)

    result = unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(Controls))
    raise SystemExit(0 if result.wasSuccessful() else 1)


def main():
    args = sys.argv[1:]
    if args == ['--self-test']:
        self_test()
    mode = args.pop(0) if args and args[0] in {'--child', '--test'} else ''
    if os.environ.get('NGFW_TRAFFIC_B') == '1' or os.environ.get('NGFW_TRAFFIC_B_REST') == '1':
        raise SystemExit('P12 root proof cannot use Wave-B namespace/REST mode')
    if not mode:
        # Dedicated slot and full lab environment must already be manager assigned.
        if os.environ.get('NGFW_SLOT') != '14' or os.environ.get('NGFW_TEST_PREFIX') != 'w14':
            raise SystemExit('P12 recovery requires assigned slot 14 / w14')
        with open('/run/lock/ngfw-acceptance-slot14.lock', 'a') as slot_lock:
            fcntl.flock(slot_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            print('P12_SLOT14_EXCLUSIVE=ACQUIRED', flush=True)
            before = snapshot()
            env = dict(os.environ, NGFW_P12_HOST_NETNS=os.readlink('/proc/self/ns/net'))
            print('P12_SHARED_BEFORE=' + json.dumps(before), flush=True)
            try:
                result = subprocess.call(['unshare', '--net', '--mount', '--propagation', 'private',
                                          sys.executable, SELF, '--child', *args], env=env)
            finally:
                after = snapshot()
                print('P12_SHARED_AFTER=' + json.dumps(after), flush=True)
                if before != after:
                    raise SystemExit('P12 shared VPP/LCP/root routes changed')
            raise SystemExit(result)
    current = os.readlink('/proc/self/ns/net')
    if current in {os.environ.get('NGFW_P12_HOST_NETNS'), os.readlink('/proc/1/ns/net')}:
        raise SystemExit('refusing P12 in shared host root namespace')
    if mode == '--test':
        # Read the actual bind mount source, rather than trusting a caller's socket flag.
        mounts = [line.split() for line in Path('/proc/self/mountinfo').read_text().splitlines()]
        sources = [row[3] for row in mounts if row[4] == '/run/vpp']
        if len(sources) != 1 or '\\' in sources[0]:
            raise SystemExit('P12 private VPP bind mount ownership missing')
        owned = Path(sources[0]) / 'api.sock'
        scratch = ROOT / '.scratch'
        if not owned.is_relative_to(scratch) or not os.path.samefile(owned, '/run/vpp/api.sock'):
            raise SystemExit('P12 API does not match owned disposable VPP mount')
        # Our parent is the isolated launcher; its actual VPP child is our sibling.
        parent = os.getppid()
        children = Path(f'/proc/{parent}/task/{parent}/children').read_text().split()
        candidates = []
        for pid in children:
            try:
                if (Path(os.readlink(f'/proc/{pid}/exe')).name == 'vpp'
                        and os.readlink(f'/proc/{pid}/ns/net') == current):
                    candidates.append(pid)
            except FileNotFoundError:
                continue
        if len(candidates) != 1:
            raise SystemExit('P12 owned disposable VPP process missing or ambiguous')
        pid = candidates[0]
        stat = Path(f'/proc/{pid}/stat').read_text()
        start = stat[stat.rindex(')') + 1:].split()[19]
        os.environ.update(NGFW_P12_PRIVATE_API=str(owned), NGFW_P12_FIB='root',
                          NGFW_P12_PRIVATE_VPP_PID=pid, NGFW_P12_PRIVATE_VPP_START=start,
                          NGFW_VPP_API_SOCKET='/run/vpp/api.sock',
                          NGFW_AGENT_VPP_API_SOCKET='/run/vpp/api.sock',
                          NGFW_VPP_CLI_SOCKET='/run/vpp/cli.sock',
                          NGFW_VPP_STATS_SOCKET='/run/vpp/stats.sock',
                          NGFW_AGENT_VPP_STATS_SOCKET='/run/vpp/stats.sock')
        print('P12_PRIVATE_VPP=' + json.dumps({'pid': pid, 'starttime': start,
                                              'netns': current}), flush=True)
        print('P12_PRIVATE_API=' + str(owned), flush=True)
        raise SystemExit(subprocess.call([str(ROOT / 'test/topology/bgp/run.sh'), *args], cwd=ROOT))
    runtime = ROOT / '.scratch' / ('p12-outer-' + str(os.getpid()))
    for name in ['netns', 'frr']:
        source = runtime / name
        source.mkdir(parents=True, mode=0o755)
        subprocess.run(['mount', '--bind', str(source), '/run/' + name], check=True)
    subprocess.run(['ip', 'link', 'set', 'lo', 'up'], check=True)
    links = json.loads(subprocess.check_output(['ip', '-d', '-j', 'address', 'show']))
    print('P12_OUTER_LINK_INVENTORY=' + json.dumps(links, sort_keys=True), flush=True)
    if not links or any(not empty_outer_link(link) for link in links):
        raise SystemExit('P12 outer network namespace is not empty')
    os.environ['NGFW_ISOLATED_TEST_RUN'] = '1'
    print('P12_PRIVATE_NETNS=' + current, flush=True)
    result = private_run([sys.executable,
                              str(ROOT / 'test/topology/hardware-smoke/isolated-vpp.py'),
                              sys.executable, SELF, '--test', *args], runtime, current)
    if list((runtime / 'netns').iterdir()) or list((runtime / 'frr').iterdir()):
        raise SystemExit('P12 private namespace/FRR cleanup incomplete')
    print('P12_OUTER_CLEANUP=PASS', flush=True)
    raise SystemExit(result)


if __name__ == '__main__':
    main()
