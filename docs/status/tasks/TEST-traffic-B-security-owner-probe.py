import os,sys,tempfile,socket,threading,time
from pathlib import Path
from unittest.mock import patch
sys.path.insert(0,str(Path('test/topology/traffic-b').resolve()))
import stack
from scenario import Refused
from pgrelay import Relay
with tempfile.TemporaryDirectory() as folder:
    root=Path(folder);address=root/'agent.sock'
    with socket.socket(socket.AF_UNIX) as server:
        server.bind(str(address));server.listen(8)
        real_link=os.readlink
        def observed(path):
            if str(path).endswith('/exe'):return '/tmp/owned.test'
            return real_link(path)
        with patch.dict(os.environ,NGFW_TRAFFIC_EXPECTED_AGENT_PID=str(os.getpid()),NGFW_TRAFFIC_VERIFIED_OWNER='w27'),patch('stack.os.getppid',return_value=os.getpid()),patch('stack.os.readlink',side_effect=observed):
            assert stack.attached_identity(address,'w27')['pid']==os.getpid()
            print('protected actual SO_PEERCRED positive: PASS (fixture executable observation mocked)')
            try:stack.attached_identity(address,'foreign');raise AssertionError('foreign owner accepted')
            except Refused:print('foreign observed owner refusal: PASS')
            with patch.dict(os.environ,NGFW_TRAFFIC_EXPECTED_AGENT_PID=str(os.getpid()+1)):
                try:stack.attached_identity(address,'w27');raise AssertionError('foreign pid accepted')
                except Refused:print('foreign/shared PID refusal: PASS')
            def foreign(path):
                if str(path)==f'/proc/{os.getpid()}/ns/mnt':return 'mnt:[foreign]'
                return observed(path)
            # pid=self means separate path distinction requires explicit own baseline mock.
            original=real_link('/proc/self/ns/mnt')
            with patch('stack.os.readlink',side_effect=foreign):
                try:stack.attached_identity(address,'w27');raise AssertionError('foreign namespace accepted')
                except Refused:print('foreign mount namespace refusal: PASS')
        os.chmod(root,0o777)
        try:stack.attached_identity(address,'w27');raise AssertionError('unprotected parent accepted')
        except Refused:print('unprotected socket parent refusal: PASS')
        os.chmod(root,0o700)
    remote,echo=socket.socketpair();seen=[]
    def echo_once():
        payload=echo.recv(4096);seen.append(payload);echo.sendall(payload);echo.close()
    worker=threading.Thread(target=echo_once);worker.start()
    def fixed_endpoint(address,timeout):
        assert address==('127.0.0.1',5432) and timeout==10
        return remote
    with patch('pgrelay.socket.create_connection',side_effect=fixed_endpoint):
        relay=Relay(root/'relay')
        assert relay.directory.stat().st_mode&0o777==0o700
        assert relay.path.stat().st_mode&0o777==0o600
        with socket.socket(socket.AF_UNIX) as client:
            client.settimeout(3);client.connect(str(relay.path));client.sendall(b'R2-private-no-secret-probe');assert client.recv(4096)==b'R2-private-no-secret-probe'
        worker.join(3);assert not worker.is_alive();relay.close();assert not relay.directory.exists()
        print('owned relay0700/socket0600, fixed endpoint, byte transfer and cleanup: PASS')
