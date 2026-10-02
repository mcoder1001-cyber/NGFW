"""Protected temporary transaction fixtures; no manager authority or host observation."""
from dataclasses import asdict, replace
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from scenario import Refused
from transaction import Observation, validate_transaction

BOOT='12345678-1234-1234-1234-123456789abc'


class Transaction(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory();self.root=Path(self.temp.name);self.root.chmod(0o700)
        self.lease=dict(task='TEST-traffic-A',slot=3,prefix='w3',boot_id=BOOT,expires_unix=200,lease_id='a'*32)
        self.observed={side:Observation(f'ns-w3-{side}',f'w3{"l" if side=="lan" else "w"}1',4,10+index,20+index,f'02:00:00:00:00:0{index+1}') for index,side in enumerate(('lan','wan'))}
        self.binding=dict(origin='source_fixture',lease_id='a'*32,run_id='b'*32,candidate_sha256='c'*64,revision=1,sides={side:asdict(value) for side,value in self.observed.items()})
        self.write('traffic-a-lease.json',self.lease);self.write('capture-binding.json',self.binding)

    def tearDown(self):self.temp.cleanup()

    def write(self,name,value):
        path=self.root/name;path.write_text(json.dumps(value));path.chmod(0o600)

    def run_fixture(self,**kwargs):
        return validate_transaction(self.root,3,'b'*32,BOOT,observer=kwargs.pop('observer',lambda slot,side:self.observed[side]),fixture=True,clock=kwargs.pop('clock',lambda:100),candidate_sha256='c'*64,revision=1,**kwargs)

    def test_fixture_consistency_never_mints_authority(self):
        result=self.run_fixture();self.assertEqual(result['status'],'FIXTURE_TRANSACTION_CONSISTENT')
        for key in ('whole_chain_proven','packet_outcomes_proven','live_provenance_verified'):self.assertFalse(result[key])
        with patch('transaction.os.open',side_effect=AssertionError('live touched filesystem')):
            with self.assertRaisesRegex(Refused,'NOTIMPLEMENTED'):
                validate_transaction(self.root,3,'b'*32,BOOT,observer=lambda *_:self.fail('live observed host'))

    def test_foreign_lease_and_candidate_refused_before_observation(self):
        for field,value in (('slot',4),('task','other'),('boot_id','foreign'),('expires_unix',99),('expires_unix',float('nan')),('prefix','w4')):
            with self.subTest(field=field):
                self.write('traffic-a-lease.json',dict(self.lease,**{field:value}))
                with self.assertRaises(Refused):self.run_fixture(observer=lambda *_:self.fail('invalid lease observed'))
        self.write('traffic-a-lease.json',self.lease)
        for field,value in (('origin','tcpdump_live'),('run_id','d'*32),('lease_id','e'*32),('revision',True),('candidate_sha256','d'*64)):
            with self.subTest(field=field):
                self.write('capture-binding.json',dict(self.binding,**{field:value}))
                with self.assertRaises(Refused):self.run_fixture(observer=lambda *_:self.fail('invalid binding observed'))

    def test_identity_changes_and_untyped_observation_refused(self):
        for value in (dict(asdict(self.observed['lan'])),replace(self.observed['lan'],ifindex=999),replace(self.observed['lan'],namespace='ns-w4-lan')):
            with self.subTest(value=value),self.assertRaises(Refused):self.run_fixture(observer=lambda slot,side:value if side=='lan' else self.observed[side])
        count={side:0 for side in ('lan','wan')}
        def changed(slot,side):
            count[side]+=1
            return replace(self.observed[side],namespace_inode=99) if count[side]>1 else self.observed[side]
        with self.assertRaisesRegex(Refused,'changed during'):self.run_fixture(observer=changed)

    def test_lease_revocation_binding_change_and_expiry_during_observation(self):
        for name in ('traffic-a-lease.json','capture-binding.json'):
            def changed(slot,side):
                path=self.root/name
                path.write_text(path.read_text()+' ')
                return self.observed[side]
            with self.subTest(name=name),self.assertRaisesRegex(Refused,'revoked or changed'):self.run_fixture(observer=changed)
            self.write('traffic-a-lease.json',self.lease);self.write('capture-binding.json',self.binding)
        ticks=iter((100,201))
        with self.assertRaisesRegex(Refused,'expired'):self.run_fixture(clock=lambda:next(ticks))

    def test_private_directory_file_alias_symlink_fifo_and_bounds(self):
        path=self.root/'traffic-a-lease.json'
        self.root.chmod(0o755)
        with self.assertRaises(Refused):self.run_fixture()
        self.root.chmod(0o700)
        with patch('transaction.os.geteuid',return_value=self.root.stat().st_uid+1):
            with self.assertRaises(Refused):self.run_fixture()
        for mode in (0o644,0o666):
            path.chmod(mode)
            with self.assertRaises(Refused):self.run_fixture()
        path.chmod(0o600)
        alias=self.root/'alias';os.link(path,alias)
        with self.assertRaises(Refused):self.run_fixture()
        alias.unlink();path.unlink();path.symlink_to(self.root/'capture-binding.json')
        with self.assertRaises(Refused):self.run_fixture()
        path.unlink();os.mkfifo(path,0o600)
        with self.assertRaises(Refused):self.run_fixture()
        path.unlink();path.write_bytes(b'x'*8193);path.chmod(0o600)
        with self.assertRaises(Refused):self.run_fixture()

    def test_duplicate_json_and_same_namespace_alias_refused(self):
        path=self.root/'traffic-a-lease.json';path.write_text(json.dumps(self.lease)[:-1]+',"slot":3}')
        with self.assertRaisesRegex(Refused,'duplicate'):self.run_fixture()
        self.write('traffic-a-lease.json',self.lease)
        self.observed['wan']=replace(self.observed['wan'],namespace_inode=self.observed['lan'].namespace_inode)
        self.binding['sides']['wan']=asdict(self.observed['wan']);self.write('capture-binding.json',self.binding)
        with self.assertRaisesRegex(Refused,'alias one namespace'):self.run_fixture()

    def test_same_descriptor_detects_mutation_before_json_parse(self):
        original=os.read;calls=[]
        def changed(fd,size):
            if not calls:
                path=self.root/'traffic-a-lease.json'
                with path.open('ab') as stream:stream.write(b'changed')
            calls.append(size)
            return original(fd,size)
        with patch('transaction.os.read',side_effect=changed),patch('transaction.json.loads',side_effect=AssertionError('changed bytes parsed')):
            with self.assertRaisesRegex(Refused,'changed while acquired'):self.run_fixture()
