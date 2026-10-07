"""Harmless external sentinels; no installer/network calls delegated to host."""
import importlib.util
import json
import os
from pathlib import Path
spec = importlib.util.spec_from_file_location('fixture', 'scripts/tests/td19-safe-root-fixtures.py')
module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module)
f = module.Fixture()
try:
    f.existing_go()
    marker = f.base / 'external-go-effect'
    target = f.root / 'usr/local/go/bin/go'
    target.write_text('#!/usr/bin/python3\nimport pathlib,sys\npathlib.Path(' + repr(str(marker)) + ').write_text("EXECUTED unchecked rooted Go")\nif sys.argv[1] == "version": print("go version go1.26.0 linux/amd64")\n')
    result = f.run('20-install-build.sh')
    print(json.dumps({'case':'existing-go-shadow', 'returncode':result.returncode, 'external_marker':marker.exists(),'calls':[x[0] for x in f.calls()]}))
finally:
    f.close()
f = module.Fixture()
try:
    marker = f.base / 'external-effect-command'
    (f.stubs / 'apt-get').write_text('#!/usr/bin/python3\nimport pathlib\npathlib.Path(' + repr(str(marker)) + ').write_text("accepted unverified effect executable")\n')
    f.existing_go()
    result = f.run('20-install-build.sh')
    print(json.dumps({'case':'regular-nonrecording-command', 'returncode':result.returncode,'external_marker':marker.exists()}))
finally:
    f.close()
