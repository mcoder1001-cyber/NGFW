#!/usr/bin/env python3
"""Read-only fixed-disk size inventory; refuse uncertain values before installation."""
import re
import subprocess
import sys

MIN_BYTES = 96 * 1024 ** 3
MAX_BYTES = 2 ** 64 - 1  # lsblk byte count: unsigned 64-bit, compared without float conversion.


def largest_fixed_disk(raw):
    largest = 0
    for line in raw.splitlines():
        fields = line.split()
        if len(fields) != 3:
            raise ValueError('malformed size inventory row')
        size, kind, removable = fields
        if not re.fullmatch(r'[0-9]{1,20}', size) or int(size) > MAX_BYTES:
            raise ValueError('disk byte count is not an unsigned 64-bit integer')
        if not re.fullmatch(r'[A-Za-z0-9_-]+', kind) or removable not in ('0', '1'):
            raise ValueError('malformed disk type/removable flag')
        if kind == 'disk' and removable == '0':
            largest = max(largest, int(size))
    if largest < MIN_BYTES:
        raise ValueError('no fixed disk meets the minimum 96 GiB layout size')
    return largest


def main():
    try:
        inventory = subprocess.run(['lsblk', '-bdnro', 'SIZE,TYPE,RM'], check=True,
                                   capture_output=True, text=True)
        print(largest_fixed_disk(inventory.stdout))
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        # Never echo failed lsblk partial output or raw command diagnostics.
        reason = str(error) if isinstance(error, ValueError) else 'cannot read disk size inventory'
        print('NGFW disk size verification failed: ' + reason, file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
