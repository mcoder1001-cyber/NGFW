#!/usr/bin/env python3
"""Live IPv4 topology acceptance, never a mock. Run from a dual-source lab client."""
import argparse
import http.client
import ipaddress
import json
import os
import socket
import ssl
import subprocess
import time
import urllib.parse


def request(origin, path, address=None, data=None, token=None):
    url = urllib.parse.urlsplit(origin)
    cls = http.client.HTTPSConnection if url.scheme == "https" else http.client.HTTPConnection
    options = {"timeout": 3, "source_address": (address, 0) if address else None}
    if url.scheme == "https":
        options["context"] = ssl.create_default_context()
    conn = cls(url.hostname, url.port, **options)
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    try:
        conn.request("POST" if data is not None else "GET", url.path.rstrip("/") + path,
                     json.dumps(data) if data is not None else None, headers)
        response = conn.getresponse()
        raw = response.read()
        return response.status, json.loads(raw) if raw else None
    finally:
        conn.close()


def probe(address, host, port):
    try:
        with socket.create_connection((host, port), timeout=2, source_address=(address, 0)):
            return True
    except OSError:
        return False


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api", required=True, help="Control origin, e.g. https://ngfw.example/api/v1")
    parser.add_argument("--control-address", required=True, help="Allowlisted controller source")
    parser.add_argument("--client-address", required=True, help="Unallowlisted attacker source")
    parser.add_argument("--allow-address", required=True, help="A separate allowlisted test source")
    parser.add_argument("--local-host", required=True)
    parser.add_argument("--local-port", type=int, default=443)
    parser.add_argument("--through-host", required=True, help="Reachable TCP server behind VPP forwarding")
    parser.add_argument("--through-port", type=int, required=True)
    parser.add_argument("--detector", choices=("webLogin", "ssh"), default="webLogin")
    parser.add_argument("--ssh-port", type=int, default=22)
    parser.add_argument("--ssh-key", help="Dedicated disposable test public-key identity (SSH mode)")
    parser.add_argument("--ssh-known-hosts", help="Pinned host-key file (SSH mode)")
    parser.add_argument("--removal", choices=("expiry", "manual"), default="expiry")
    parser.add_argument("--block-sec", type=int, required=True, help="Configured first-offence blockSec")
    args = parser.parse_args()
    if args.detector == "ssh" and (not args.ssh_key or not args.ssh_known_hosts):
        parser.error("SSH mode requires --ssh-key and --ssh-known-hosts")
    token = os.environ.get("NGFW_TEST_API_TOKEN")
    if not token:
        parser.error("NGFW_TEST_API_TOKEN is required for control-plane state")
    for address in (args.control_address, args.client_address, args.allow_address):
        if ipaddress.ip_address(address).version != 4:
            parser.error("This topology driver requires IPv4 source bindings")
    if len({args.control_address, args.client_address, args.allow_address}) != 3:
        parser.error("Use three distinct source addresses")
    if args.block_sec < 10:
        parser.error("Use blockSec >= 10 for packet probing")

    def blocked(address):
        status, payload = request(args.api, "/state/auto-block", args.control_address, token=token)
        if status != 200:
            raise AssertionError("control state unavailable: " + str(status))
        # Endpoint returns {items:[...]}; no runtime/config data is printed.
        entries = payload["items"]
        return any(ipaddress.ip_interface(e["source"]).ip == ipaddress.ip_address(address) for e in entries)

    def login_failures(address):
        for _ in range(10):
            if args.detector == "ssh":
                result = subprocess.run([
                    "ssh", "-b", address, "-p", str(args.ssh_port), "-i", args.ssh_key,
                    "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes",
                    "-o", "PreferredAuthentications=publickey", "-o", "StrictHostKeyChecking=yes",
                    "-o", "UserKnownHostsFile=" + args.ssh_known_hosts,
                    "-o", "ConnectTimeout=3", "-o", "ConnectionAttempts=1",
                    "NGFW_TEST_AUTOBLOCK_NONEXISTENT@" + args.local_host, "true",
                ], stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, timeout=8, check=False)
                if result.returncode == 0:
                    raise AssertionError("test SSH identity unexpectedly authenticated")
                # Routing, bind and host-key failures are not failed logins.
                if b"Permission denied" not in result.stderr and not blocked(address):
                    raise AssertionError("SSH attempt failed before authentication")
                continue
            try:
                status, _ = request(args.api, "/auth/login", address,
                                    {"username": "NGFW_TEST_AUTOBLOCK_NONEXISTENT", "password": "NGFW_TEST_PASSWORD_AUTOBLOCK"})
                if status != 401:
                    raise AssertionError("login did not reach authentication: " + str(status))
            except (OSError, http.client.HTTPException):
                # Threshold may have already cut off the last attempt.
                pass

    assert not blocked(args.client_address), "Start with a clean first-offence client"
    assert probe(args.client_address, args.local_host, args.local_port), "local-in baseline unavailable"
    assert probe(args.client_address, args.through_host, args.through_port), "forwarding baseline unavailable"
    login_failures(args.client_address)
    deadline = time.monotonic() + 10
    while not blocked(args.client_address) and time.monotonic() < deadline:
        time.sleep(0.25)
    assert blocked(args.client_address), "10 bad logins did not block source"
    assert not probe(args.client_address, args.local_host, args.local_port), "local-in was not blocked"
    assert not probe(args.client_address, args.through_host, args.through_port), "forwarding was not blocked"
    print("PASS: threshold blocked local-in and through traffic")
    if args.removal == "manual":
        status, payload = request(args.api, "/actions/auto-block/unblock", args.control_address,
                                  {"source": args.client_address}, token)
        assert status == 200 and payload.get("unblocked") is True, "manual removal failed"
    deadline = time.monotonic() + (10 if args.removal == "manual" else args.block_sec + 40)
    while blocked(args.client_address) and time.monotonic() < deadline:
        time.sleep(1)
    assert not blocked(args.client_address), "block was not removed"
    deadline = time.monotonic() + 10
    while not probe(args.client_address, args.through_host, args.through_port) and time.monotonic() < deadline:
        time.sleep(0.25)
    assert probe(args.client_address, args.local_host, args.local_port), "local-in did not reopen"
    assert probe(args.client_address, args.through_host, args.through_port), "forwarding did not reopen"
    print("PASS: " + args.removal + " reopened local-in and through traffic")
    login_failures(args.allow_address)
    time.sleep(2)
    assert not blocked(args.allow_address), "allowlisted test source was blocked"
    assert probe(args.allow_address, args.local_host, args.local_port), "allowlisted local-in lost access"
    assert probe(args.allow_address, args.through_host, args.through_port), "allowlisted forwarding lost access"
    print("PASS: allowlisted source remained permitted")


if __name__ == "__main__":
    main()
