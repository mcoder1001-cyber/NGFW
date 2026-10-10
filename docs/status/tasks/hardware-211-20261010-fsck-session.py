#!/usr/bin/env python3
"""Controller PTY driver. Invoke only after manager's corrective-phase release."""
import os
import pathlib
import select
import subprocess
import sys

PRIVATE = pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211')
TRANSCRIPT = PRIVATE / 'repair-interactive-transcript.raw'
os.umask(0o077)
assert os.geteuid() == 0 and PRIVATE.stat().st_mode & 0o777 == 0o700
assert not os.path.lexists(TRANSCRIPT)
capacity = os.statvfs(PRIVATE)
assert capacity.f_bfree * capacity.f_frsize > 1073741824 + 536870912
ssh = ['ssh', '-tt', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes',
       '-o', 'HostKeyAlias=172.30.110.211', '-o', 'ConnectTimeout=10', '-p', '2222',
       'root@172.30.110.211',
       '/bin/bash --noprofile --norc /root/recovery-private/repair-wrapper.sh --repair']
with TRANSCRIPT.open('xb') as transcript:
    child = subprocess.Popen(ssh, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                             stderr=subprocess.STDOUT)
    pending = b''
    count = 0
    input_open = True
    while True:
        streams = [child.stdout] + ([sys.stdin] if input_open else [])
        ready, _, _ = select.select(streams, [], [], 1)
        if child.stdout in ready:
            data = os.read(child.stdout.fileno(), 65536)
            if not data:
                break
            transcript.write(data)
            transcript.flush()
            os.fsync(transcript.fileno())
            count += len(data)
            pending += data
            if b'?' in pending:
                # Full text stays private. Manager/worker reads the transcript
                # to classify the exact prompt before sending a single answer.
                print('PENDING_PROMPT private_bytes=' + str(count), flush=True)
                pending = b''
            elif len(pending) > 65536:
                pending = pending[-4096:]
        if sys.stdin in ready:
            line = sys.stdin.readline()
            if not line:
                input_open = False
                child.stdin.write(b'\x03')
                child.stdin.flush()
                print('INPUT_CLOSED_INTERRUPT_SENT', flush=True)
                continue
            answer = line.strip()
            if answer in ('y', 'n'):
                # e2fsck ask_yn reads one byte with ICANON disabled. A newline
                # would accept the following prompt's default unintentionally.
                child.stdin.write(answer.encode())
                child.stdin.flush()
                print('SINGLE_ANSWER_SENT ' + answer, flush=True)
            elif answer == 'stop':
                child.stdin.write(b'\x03')
                child.stdin.flush()
                print('INTERRUPT_SENT', flush=True)
            else:
                print('REFUSED_INPUT use y/n/stop only', flush=True)
    status = child.wait()
    print('SESSION_EXIT=' + str(status) + ' private_bytes=' + str(count), flush=True)
    sys.exit(status)
