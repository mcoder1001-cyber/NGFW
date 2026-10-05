#!/bin/sh
set -eu
HERE=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
export PYTHONDONTWRITEBYTECODE=1
for tool in python3 openssl zstd grub-editenv grub-reboot; do
  command -v "$tool" >/dev/null 2>&1 || { echo "upgrade tests: required tool missing: $tool" >&2; exit 1; }
done
python3 -m unittest discover -s "$HERE" -p 'test_*.py' -v
