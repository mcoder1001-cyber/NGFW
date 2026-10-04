#!/usr/bin/env python3
"""Fixed rig peers: bounded echo servers and deterministic client flow inventory."""
import argparse
import json
import socket
import threading
import sys
sys.dont_write_bytecode = True
from scenario import slot_values


def client(slot, mode):
    results = []
    probes = [(8000, 40000 + index, True) for index in range(20)]
    probes += [(8001, 41000 + index, True) for index in range(5)]
    probes += [(8002, 42000 + index, False) for index in range(3)]
    if mode == 'ei':
        probes += [(8000, 48000, True), (8001, 48000, True)]
    for port, source, expected in probes:
        with socket.socket() as channel:
            channel.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            channel.settimeout(2)
            channel.bind((f'10.{slot}.10.2', source))
            success = False
            try:
                channel.connect((f'10.{slot}.99.1', port))
                channel.sendall(b'traffic-a')
                success = channel.recv(9) == b'traffic-a'
            except (TimeoutError, ConnectionError, OSError):
                pass
            results.append({'port': port, 'source': source, 'expected': expected, 'success': success})
    print(json.dumps(results), flush=True)
    return int(any(probe['success'] != probe['expected'] for probe in results))


def server(slot):
    def listen(port):
        with socket.socket() as listener:
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            listener.bind((f'10.{slot}.99.1', port))
            listener.listen(32)
            print('READY:' + str(port), flush=True)
            while True:
                connection, _ = listener.accept()
                with connection:
                    connection.settimeout(3)
                    try:
                        payload = connection.recv(9)
                        if payload:
                            connection.sendall(payload)
                    except (OSError, TimeoutError):
                        pass
    workers = [threading.Thread(target=listen, args=(port,), daemon=True) for port in (8000, 8001, 8002)]
    for worker in workers:
        worker.start()
    for worker in workers:
        worker.join()


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('role', choices=('client', 'server'))
    parser.add_argument('--slot', required=True, type=int)
    parser.add_argument('--mode', choices=('ed', 'ei'), default='ed')
    arguments = parser.parse_args()
    slot_values(arguments.slot)
    if arguments.role == 'server':
        server(arguments.slot)
    else:
        raise SystemExit(client(arguments.slot, arguments.mode))
