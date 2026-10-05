#!/usr/bin/env python3
"""One persistent TCP connection; challenge echo exposes its translated peer tuple."""
import argparse
import json
import secrets
import socket
import sys


def line(stream):
    raw = stream.readline(4097)
    if not raw or len(raw) > 4096 or not raw.endswith(b'\n'):
        raise RuntimeError('missing or oversized flow frame')
    return json.loads(raw)


class Flow:
    def __init__(self, inside, host, port, timeout=10):
        self.socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.socket.settimeout(timeout)
        self.socket.bind((inside, 0))
        self.socket.connect((host, port))
        self.stream = self.socket.makefile('rb')
        self.local = self.socket.getsockname()
        self.challenge = secrets.token_hex(16)
        self.sequence = 0
        self.peer = None

    def exchange(self):
        self.sequence += 1
        message = {'challenge': self.challenge, 'sequence': self.sequence}
        self.socket.sendall((json.dumps(message) + '\n').encode())
        reply = line(self.stream)
        if reply.get('challenge') != self.challenge or reply.get('sequence') != self.sequence:
            raise RuntimeError('flow challenge/sequence mismatch')
        peer = reply.get('peer')
        if not isinstance(peer, list) or len(peer) != 2:
            raise RuntimeError('missing observed translated tuple')
        if self.peer is not None and peer != self.peer:
            raise RuntimeError('translated tuple changed on established flow')
        self.peer = peer
        return {'inside': list(self.local), 'outside': peer, 'sequence': self.sequence}

    def close(self):
        self.stream.close()
        self.socket.close()


def serve(listener):
    # Deliberately accept exactly once: reconnection can never count as continuity.
    channel, peer = listener.accept()
    with channel, channel.makefile('rb') as stream:
        channel.settimeout(90)
        while True:
            try:
                request = line(stream)
            except RuntimeError:
                return
            request['peer'] = list(peer)
            channel.sendall((json.dumps(request) + '\n').encode())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--server', action='store_true')
    parser.add_argument('--address', required=True)
    parser.add_argument('--port', required=True, type=int)
    parser.add_argument('--inside')
    args = parser.parse_args()
    if args.server:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
            listener.bind((args.address, args.port))
            listener.listen(1)
            serve(listener)
        return
    if not args.inside:
        parser.error('--inside is required for the client')
    flow = Flow(args.inside, args.address, args.port)
    try:
        for command in sys.stdin:
            if command.strip() != 'exchange':
                raise RuntimeError('unknown flow operation')
            print(json.dumps(flow.exchange()), flush=True)
    finally:
        flow.close()


if __name__ == '__main__':
    main()
