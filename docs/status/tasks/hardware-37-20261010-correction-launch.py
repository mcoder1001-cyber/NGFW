#!/usr/bin/env python3
"""Allocated controller PTY -> fresh reviewed offline audit -> fixed driver."""
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import subprocess
import sys

HERE=pathlib.Path(__file__).resolve().parent
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37')


def main():
    os.umask(0o077)
    assert sys.stdin.isatty(),'refuse redirected/closed controller input before any target action'
    assert os.geteuid()==0 and PRIVATE.stat().st_mode&0o777==0o700
    source=HERE/'hardware-37-20261010-offline-preserve.py'
    assert hashlib.sha256(source.read_bytes()).hexdigest()=='bdb2a28a5235142dc60c3ce72f2577acdee7ead4667f19b3733c22acef4c4301'
    driver=HERE/'hardware-37-20261010-fsck-session.py'
    assert hashlib.sha256(driver.read_bytes()).hexdigest()=='5f45bf37db7d5e066165b07f66bbbe55a4e383aadd3f483525ba50222e3ed973'
    failed=PRIVATE/'repair-interactive-transcript.raw'
    archive=PRIVATE/'repair-interactive-transcript-launch-eof.raw'
    assert failed.is_file() and not failed.is_symlink() and failed.stat().st_size==39
    assert hashlib.sha256(failed.read_bytes()).hexdigest()=='29e578c3a5e15a1d44d3b9659e0a3344d13bccb7680c63f5fe5a727641efe34a'
    assert not os.path.lexists(archive)
    os.rename(failed,archive)
    fd=os.open(PRIVATE,os.O_RDONLY|os.O_DIRECTORY)
    try:os.fsync(fd)
    finally:os.close(fd)
    spec=importlib.util.spec_from_file_location('host37preserve',source)
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
    fields={'REMOTE_PARENT':module.REMOTE_PARENT,'IMAGE':module.IMAGE,'BLOCKS':module.BLOCKS,
            'PRESERVE':False,'DIAGNOSE':False}
    extra="\nassert not os.path.lexists(parent/'root-repair.undo') and not os.path.lexists(parent/'undo-write-preflight.test')\n"
    p=subprocess.run(module.SSH+['python3 -B -'],input=module.code(fields,module.IDENTITY+extra+module.REMOTE).encode(),
                     capture_output=True,timeout=240)
    receipt=module.save('pre-correction-offline-audit-interactive-20261010.json',p.stdout)
    error=module.save('pre-correction-offline-audit-interactive-20261010.stderr',p.stderr)
    assert p.returncode==0 and not p.stderr,'fresh audit failed; no corrective driver started'
    data=json.loads(p.stdout)
    previous=json.loads((PRIVATE/'offline-preservation-20261010.json').read_text())
    assert data['status']=='PASS_AUDIT_ONLY'
    assert data['health_before']['ioerr_counter']==previous['health_after']['ioerr_counter']
    pattern=r'(?i)(ata\d.*(error|failed|reset|timeout|unc)|scsi.*(error|failed|reset|timeout)|I/O error|Buffer I/O|blk_update_request|end_request|uncorrectable|critical medium error)'
    def events(text):return [line for line in text.splitlines() if re.search(pattern,line)]
    assert not [line for line in events(data['health_before']['kernel']) if line not in events(previous['health_after']['kernel'])]
    print(json.dumps({'fresh_pre_fs_audit':'PASS','receipt':receipt,'stderr':error,
                      'counter':data['health_after']['ioerr_counter'],'newstorage':len(data['new_storage_error_lines']),
                      'controller_stdin_isatty':True,'failed39B_preserved':True,'next':'fixed5f45driver'}),flush=True)
    os.execv('/usr/bin/python3',['python3',str(driver)])


if __name__=='__main__':
    main()
