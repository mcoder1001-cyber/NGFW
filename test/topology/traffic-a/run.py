#!/usr/bin/env python3
"""Read-only plan and a deliberately fail-closed future live entry point."""
import argparse
import json
import os
from pathlib import Path
import sys

sys.dont_write_bytecode = True
from scenario import Refused, plan, require_implemented, validate_environment, validate_lease


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=('plan', 'run'))
    parser.add_argument('--slot', type=int, required=True)
    arguments = parser.parse_args()
    try:
        if arguments.mode == 'plan':
            print(json.dumps(plan(arguments.slot), indent=2))
            return 0
        validate_environment(os.environ, arguments.slot)
        # Refuse incomplete source before locks, host reads or any command.
        require_implemented()
        validate_lease(Path(f'/run/ngfw-test/w{arguments.slot}/traffic-a-lease.json'),
                       arguments.slot, Path('/proc/sys/kernel/random/boot_id').read_text().strip())
        raise Refused('NOTIMPLEMENTED: locked composed transaction/capture lifecycle')
    except (Refused, OSError, ValueError) as error:
        print(f'TEST-traffic-A refused: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
