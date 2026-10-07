#!/usr/bin/env python3
"""Offline newc payload substitution; never launches a VM or extracts guest paths."""
import argparse
import gzip
import hashlib
import json
from pathlib import Path

IMAGE_SHA = "87efa2ed9017cf93be4cefd71e01cfe8eafaee277a8246a60370c1a1f11df1f4"
BINARIES = {
    "usr/sbin/ngfw-agent": "ngfw-agent",
    "usr/lib/ngfw/ngfw-ra-namespace-broker": "ngfw-ra-namespace-broker",
    "usr/lib/ngfw/ngfw-ra-daemon": "ngfw-ra-daemon",
}


def entries(data):
    position = 0
    seen = set()
    while position + 110 <= len(data):
        header = data[position:position + 110]
        if header[:6] != b"070701":
            raise ValueError("unsupported archive header")
        fields = [int(header[6 + i * 8:14 + i * 8], 16) for i in range(13)]
        namesize, size = fields[11], fields[6]
        if namesize < 1:
            raise ValueError("invalid archive name length")
        name_end = position + 110 + namesize
        raw_name = data[position + 110:name_end]
        if len(raw_name) != namesize or raw_name[-1:] != b"\0":
            raise ValueError("truncated archive name")
        name = raw_name[:-1].decode("utf-8")
        payload_start = (name_end + 3) & ~3
        payload_end = payload_start + size
        end = (payload_end + 3) & ~3
        if end > len(data) or name in seen:
            raise ValueError("truncated or duplicate archive entry")
        seen.add(name)
        yield name, fields, data[payload_start:payload_end], data[position:end]
        position = end
        if name == "TRAILER!!!":
            if any(data[position:]):
                raise ValueError("unexpected archive tail")
            return
    raise ValueError("missing archive trailer")


def replace_record(name, fields, payload):
    fields = fields.copy()
    fields[6] = len(payload)
    header = b"070701" + b"".join(f"{field:08x}".encode() for field in fields)
    prefix = header + name.encode() + b"\0"
    prefix += b"\0" * (-len(prefix) % 4)
    record = prefix + payload
    return record + b"\0" * (-len(record) % 4)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("image", type=Path)
    parser.add_argument("--binaries", type=Path)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    compressed = args.image.read_bytes()
    if hashlib.sha256(compressed).hexdigest() != IMAGE_SHA:
        raise ValueError("preserved Boot38 image pin mismatch")
    data = gzip.decompress(compressed)
    records = list(entries(data))
    selected = {name: (fields, payload) for name, fields, payload, _ in records}
    pins = {}
    changes = {}
    for path, binary in BINARIES.items():
        fields, old = selected[path]
        if fields[1] != 0o100755 or fields[2:5] != [0, 0, 1]:
            raise ValueError("original binary metadata mismatch")
        payload = (args.binaries / binary).read_bytes() if args.binaries else old
        if payload[:4] != b"\x7fELF":
            raise ValueError("replacement is not an ELF")
        digest = hashlib.sha256(payload).hexdigest()
        pins[binary] = {"sha256": digest, "size": len(payload)}
        if args.binaries:
            changes[path] = payload
        if path.startswith("usr/lib/ngfw/"):
            receipt = path + ".sha256"
            receipt_fields, old_receipt = selected[receipt]
            if receipt_fields[1] != 0o100644 or receipt_fields[2:5] != [0, 0, 1]:
                raise ValueError("original receipt metadata mismatch")
            if old_receipt != (hashlib.sha256(old).hexdigest() + "\n").encode():
                raise ValueError("original binary receipt mismatch")
            if args.binaries:
                changes[receipt] = (digest + "\n").encode()
    print(json.dumps({"image_input_sha256": IMAGE_SHA, "binaries": pins}, sort_keys=True))
    if bool(args.binaries) != bool(args.output):
        raise ValueError("binaries and new output must be supplied together")
    if args.output:
        rebuilt = b"".join(replace_record(name, fields, changes[name]) if name in changes else raw
                           for name, fields, _, raw in records)
        # Reparse the result and prove every other payload AND metadata unchanged.
        after = list(entries(rebuilt))
        if len(after) != len(records):
            raise ValueError("archive entry count changed")
        for before, new in zip(records, after):
            if before[0] != new[0] or before[0] not in changes and before != new:
                raise ValueError("unowned archive entry changed")
            if before[0] in changes:
                expected_fields = before[1].copy()
                expected_fields[6] = len(changes[before[0]])
                if new[1] != expected_fields or new[2] != changes[before[0]]:
                    raise ValueError("replacement metadata or payload changed")
        output = gzip.compress(rebuilt, compresslevel=1, mtime=0)
        with args.output.open("xb") as stream:
            stream.write(output)
        print(json.dumps({"image_output_sha256": hashlib.sha256(output).hexdigest(),
                          "changed_entries": sorted(changes), "compressed_bytes": len(output)}, sort_keys=True))


if __name__ == "__main__":
    main()
