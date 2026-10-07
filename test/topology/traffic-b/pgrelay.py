"""Owned Unix relay to the fixed host PostgreSQL TCP endpoint.

This preserves a protocol fixture's private network namespace. No SQL, password
or payload is interpreted/logged; access is root-only within an owned0700 dir.
"""
import os
from pathlib import Path
import selectors
import socket
import struct
import threading
import time
from scenario import Refused

class Relay:
    def __init__(self,directory):
        self.directory=Path(directory);self.directory.mkdir(mode=0o700,parents=True,exist_ok=False)
        self.path=self.directory/'.s.PGSQL.5432';self.clients=[];self.lock=threading.Lock();self.closed=threading.Event()
        self.server=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM);self.server.bind(str(self.path));os.chmod(self.path,0o600)
        self.inode=self.path.stat().st_ino;self.server.listen(8);self.server.settimeout(.2)
        self.thread=threading.Thread(target=self.accept,daemon=True);self.thread.start()
    def accept(self):
        while not self.closed.is_set():
            try:client,_=self.server.accept()
            except TimeoutError:continue
            except OSError:break
            _,uid,_=struct.unpack('3i',client.getsockopt(socket.SOL_SOCKET,socket.SO_PEERCRED,struct.calcsize('3i')))
            if uid!=os.geteuid():client.close();continue
            threading.Thread(target=self.forward,args=(client,),daemon=True).start()
    def forward(self,client):
        remote=None
        try:
            remote=socket.create_connection(('127.0.0.1',5432),timeout=10)
            with self.lock:self.clients.extend((client,remote))
            selector=selectors.DefaultSelector();selector.register(client,selectors.EVENT_READ,remote);selector.register(remote,selectors.EVENT_READ,client)
            deadline=time.monotonic()+1800
            try:
                while not self.closed.is_set() and time.monotonic()<deadline:
                    for key,_ in selector.select(.2):
                        data=key.fileobj.recv(65536)
                        if not data:return
                        key.data.sendall(data)
            finally:selector.close()
        except OSError:pass
        finally:
            for endpoint in (client,remote):
                if endpoint is not None:
                    with self.lock:
                        if endpoint in self.clients:self.clients.remove(endpoint)
                    endpoint.close()
    def close(self):
        self.closed.set();self.server.close()
        with self.lock:
            for client in self.clients:
                try:client.shutdown(socket.SHUT_RDWR)
                except OSError:pass
                client.close()
        self.thread.join(timeout=2)
        if self.path.exists():
            if self.path.stat().st_ino!=self.inode:raise Refused('foreign relay socket replacement')
            self.path.unlink()
        self.directory.rmdir()
