"""Inactive bounded capture adapter; fixtures cannot attest live namespace ownership."""
import hashlib
import os
from pathlib import Path
import re
import selectors
import signal
import stat
import subprocess
import time

from evidence import read_pcap
from scenario import Refused, STAGES, slot_values

MAX_PCAP = 16 * 1024 * 1024
MAX_ACCOUNTING = 65536


def capture_argv(slot, side):
    slot_values(slot)
    if side not in ('lan', 'wan'):
        raise Refused('invalid capture side')
    namespace = f'ns-w{slot}-{side}'
    device = f'w{slot}{"l" if side == "lan" else "w"}1'
    return ['ip', 'netns', 'exec', namespace, 'tcpdump', '-n', '-U', '-s',
            '65535', '-i', device, '-w', '-', 'icmp', 'or', 'tcp']


def process_executor(argv, *, environment):
    """Create one independently owned group with separate byte streams."""
    return subprocess.Popen(argv, env=environment, stdin=subprocess.DEVNULL,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            start_new_session=True, close_fds=True)


def produce(directory, slot, side, stage, run_id, timeout, *, executor=process_executor,
            fixture=False):
    """Produce offline stub evidence; actual manager namespace attestation is absent."""
    argv = capture_argv(slot, side)
    if (stage not in {item.name for item in STAGES} or not isinstance(run_id, str)
            or not re.fullmatch('[0-9a-f]{32}', run_id) or type(timeout) not in (int, float)
            or not 0 < timeout <= 1200):
        raise Refused('invalid bounded producer stage/run/timeout')
    if not fixture:
        raise Refused('NOTIMPLEMENTED: manager namespace/device ownership and lease transaction binding')
    # The fixture caller must explicitly supply an executor; never invoke ip here.
    if executor is process_executor:
        raise Refused('fixture requires explicit stub executor')
    try:
        root = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    except OSError as error:
        raise Refused('private capture directory cannot be safely opened') from error
    fd = None; process = None; owns_group = False; selector = selectors.DefaultSelector()
    filename = side + '.pcap'
    stderr = bytearray(); written = 0; started = time.time(); deadline = time.monotonic() + timeout
    try:
        info = os.fstat(root)
        if info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) != 0o700:
            raise Refused('capture directory must be owned0700')
        fd = os.open(filename, os.O_RDWR | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC,
                     0o600, dir_fd=root)
        process = executor(tuple(argv), environment={'PATH': '/usr/sbin:/usr/bin:/sbin:/bin', 'LC_ALL': 'C'})
        # Executor contract is a genuine isolated subprocess, not supplied statuses.
        if os.getpgid(process.pid) != process.pid or process.pid == os.getpgrp():
            raise Refused('executor did not create its own process group')
        owns_group = True
        selector.register(process.stdout, selectors.EVENT_READ, 'pcap')
        selector.register(process.stderr, selectors.EVENT_READ, 'accounting')
        while selector.get_map():
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise Refused('capture producer timed out')
            for key, _ in selector.select(min(remaining, 0.05)):
                chunk = os.read(key.fd, 65536)
                if not chunk:
                    selector.unregister(key.fileobj); continue
                if key.data == 'pcap':
                    if written + len(chunk) > MAX_PCAP:
                        raise Refused('pcap producer exceeded byte bound')
                    view = memoryview(chunk)
                    while view:
                        count = os.write(fd, view); view = view[count:]
                    written += len(chunk)
                else:
                    if len(stderr) + len(chunk) > MAX_ACCOUNTING:
                        raise Refused('capture accounting exceeded byte bound')
                    stderr.extend(chunk)
        if process.wait(timeout=max(0.001, deadline-time.monotonic())) != 0:
            raise Refused('capture producer failed')
        ended = time.time()
        text = stderr.decode('ascii', errors='strict')
        captured = re.findall(r'^([0-9]+) packets captured$', text, re.M)
        received = re.findall(r'^([0-9]+) packets received by filter$', text, re.M)
        dropped = re.findall(r'^([0-9]+) packets dropped by kernel$', text, re.M)
        if len(captured) != 1 or len(received) != 1 or dropped != ['0']:
            raise Refused('missing/duplicate/lost capture accounting')
        os.lseek(fd, 0, os.SEEK_SET)
        data = bytearray()
        while len(data) < written:
            chunk = os.read(fd, min(65536, written-len(data)))
            if not chunk: raise Refused('capture file changed during snapshot')
            data.extend(chunk)
        packets, count = read_pcap(bytes(data))
        if count != int(captured[0]) or int(received[0]) < count:
            raise Refused('capture records disagree with accounting')
        if any(not started <= item.timestamp <= ended for item in packets):
            raise Refused('capture timestamps outside producer interval')
        os.fsync(fd)
        metadata = dict(origin='source_fixture', stage=stage, run_id=run_id,
                        namespace=argv[3], device=argv[10], argv=list(argv),
                        sha256=hashlib.sha256(data).hexdigest(), started=started, ended=ended,
                        received=int(received[0]), dropped=0)
        return {'path': str(Path(directory)/filename), 'metadata': metadata,
                'status': 'FIXTURE_CAPTURED', 'whole_chain_proven': False,
                'packet_outcomes_proven': False, 'live_provenance_verified': False}
    finally:
        selector.close()
        if process is not None and owns_group:
            for sig in (signal.SIGTERM, signal.SIGKILL):
                try: os.killpg(process.pid, sig)
                except ProcessLookupError: pass
                if sig == signal.SIGTERM:
                    try: process.wait(timeout=0.2)
                    except subprocess.TimeoutExpired: pass
            process.wait(timeout=2)
        if process is not None:
            process.stdout.close(); process.stderr.close()
        if fd is not None: os.close(fd)
        os.close(root)
