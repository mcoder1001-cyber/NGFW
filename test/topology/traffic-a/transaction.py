"""Protected fixture transaction consistency; no live manager authority exists yet."""
from dataclasses import dataclass
import hashlib
import json
import math
import os
import re
import stat
import time

from scenario import Refused, slot_values


@dataclass(frozen=True)
class Observation:
    namespace: str
    device: str
    namespace_device: int
    namespace_inode: int
    ifindex: int
    mac: str


def _pairs(items):
    result = {}
    for key, value in items:
        if key in result:
            raise Refused('duplicate transaction JSON field')
        result[key] = value
    return result


def _snapshot(root, filename, owner):
    try:
        fd = os.open(filename, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC, dir_fd=root)
    except OSError as error:
        raise Refused('transaction file unavailable') from error
    try:
        before = os.fstat(fd)
        if (not stat.S_ISREG(before.st_mode) or stat.S_IMODE(before.st_mode) != 0o600
                or before.st_uid != owner or before.st_nlink != 1 or not 1 <= before.st_size <= 8192):
            raise Refused('transaction file must be owned private bounded unaliased regular file')
        data = bytearray()
        while len(data) <= 8192:
            chunk = os.read(fd, min(4096, 8193-len(data)))
            if not chunk: break
            data.extend(chunk)
        after = os.fstat(fd)
        signature = lambda info: (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns)
        if len(data) != before.st_size or signature(before) != signature(after):
            raise Refused('transaction file changed while acquired')
        try:
            value = json.loads(data, object_pairs_hook=_pairs,
                               parse_constant=lambda _: (_ for _ in ()).throw(Refused('nonfinite JSON')))
        except (ValueError, UnicodeError) as error:
            raise Refused('invalid transaction JSON') from error
        if not isinstance(value, dict): raise Refused('transaction JSON object required')
        return value, (signature(after), hashlib.sha256(data).hexdigest())
    finally:
        os.close(fd)


def _hex(value, length):
    return isinstance(value, str) and re.fullmatch('[0-9a-f]{'+str(length)+'}', value) is not None


def _observe(value, slot, side):
    if not isinstance(value, Observation): raise Refused('typed observation required')
    if value.namespace != f'ns-w{slot}-{side}' or value.device != f'w{slot}{"l" if side=="lan" else "w"}1':
        raise Refused('foreign namespace/device observation')
    if any(type(number) is not int or number <= 0 for number in
           (value.namespace_device, value.namespace_inode, value.ifindex)):
        raise Refused('positive concrete observed identity required')
    if not isinstance(value.mac, str) or not re.fullmatch('(?:[0-9a-f]{2}:){5}[0-9a-f]{2}', value.mac):
        raise Refused('invalid observed device MAC')
    return value


def validate_transaction(directory, slot, run_id, boot_id, *, observer, fixture=False, clock=time.time, candidate_sha256=None, revision=None):
    """Check temporary consistency only; matching arbitrary fixture data is no authority."""
    slot_values(slot)
    if not _hex(run_id,32) or not isinstance(boot_id,str) or not re.fullmatch('[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}',boot_id):
        raise Refused('invalid transaction run/boot identity')
    if not fixture:
        raise Refused('NOTIMPLEMENTED: authoritative manager namespace/lease/candidate transaction issuer')
    if not _hex(candidate_sha256,64) or type(revision) is not int or revision<=0:
        raise Refused('explicit expected candidate digest/revision required')
    owner=os.geteuid()
    try:
        root=os.open(directory,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW|os.O_CLOEXEC)
    except OSError as error:
        raise Refused('transaction directory unavailable') from error
    try:
        root_info=os.fstat(root)
        if root_info.st_uid!=owner or stat.S_IMODE(root_info.st_mode)!=0o700:
            raise Refused('transaction parent must be owned0700')
        lease,lease_identity=_snapshot(root,'traffic-a-lease.json',owner)
        binding,binding_identity=_snapshot(root,'capture-binding.json',owner)
        if lease_identity[0][:2]==binding_identity[0][:2]:raise Refused('lease/binding alias')
        if set(lease)!={'task','slot','prefix','boot_id','expires_unix','lease_id'}:
            raise Refused('lease fields differ from task contract')
        if (lease['task']!='TEST-traffic-A' or type(lease['slot']) is not int or lease['slot']!=slot
                or lease['prefix']!=f'w{slot}' or lease['boot_id']!=boot_id or not _hex(lease['lease_id'],32)):
            raise Refused('foreign lease task/slot/boot/id')
        if set(binding)!={'origin','lease_id','run_id','candidate_sha256','revision','sides'}:
            raise Refused('fixture binding fields differ from explicit contract')
        if (binding['origin']!='source_fixture' or binding['lease_id']!=lease['lease_id'] or binding['run_id']!=run_id
                or binding['candidate_sha256']!=candidate_sha256 or type(binding['revision']) is not int or binding['revision']!=revision
                or not isinstance(binding['sides'],dict) or set(binding['sides'])!={'lan','wan'}):
            raise Refused('unbound/foreign fixture candidate transaction')
        def expiry():
            now=clock();end=lease['expires_unix']
            if type(now) not in (int,float) or type(end) not in (int,float) or not math.isfinite(now) or not math.isfinite(end) or not now<end<=now+7200:
                raise Refused('expired/nonfinite/unbounded transaction lease')
        expiry();observations={}
        for side in ('lan','wan'):
            expected=binding['sides'][side]
            fields={'namespace','device','namespace_device','namespace_inode','ifindex','mac'}
            if not isinstance(expected,dict) or set(expected)!=fields:raise Refused('incomplete namespace binding')
            try: expected_observation=Observation(**expected)
            except TypeError as error:raise Refused('invalid observation binding') from error
            expected_observation=_observe(expected_observation,slot,side)
            observed=_observe(observer(slot,side),slot,side)
            if observed!=expected_observation:raise Refused('namespace/device identity does not match fixture binding')
            observations[side]=observed
        if (observations['lan'].namespace_device,observations['lan'].namespace_inode)==(observations['wan'].namespace_device,observations['wan'].namespace_inode):
            raise Refused('two sides cannot alias one namespace')
        for side in ('lan','wan'):
            if _observe(observer(slot,side),slot,side)!=observations[side]:raise Refused('observed namespace/device changed during validation')
        expiry()
        if (_snapshot(root,'traffic-a-lease.json',owner)[1]!=lease_identity
                or _snapshot(root,'capture-binding.json',owner)[1]!=binding_identity):
            raise Refused('lease/binding revoked or changed during validation')
        after=os.fstat(root)
        if (after.st_uid,stat.S_IMODE(after.st_mode))!=(owner,0o700):raise Refused('private parent ownership changed')
        return {'status':'FIXTURE_TRANSACTION_CONSISTENT','revision':binding['revision'],
                'whole_chain_proven':False,'packet_outcomes_proven':False,'live_provenance_verified':False}
    finally:
        os.close(root)
