"""review L1: a crafted OpenAPI document can make generation fail, but never inject code into the SDK."""
from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

GEN = Path(__file__).resolve().parent.parent / "tools" / "gen.py"


def run_gen(tmp: Path, doc: dict) -> subprocess.CompletedProcess[str]:
    spec = tmp / "openapi.json"
    spec.write_text(json.dumps(doc))
    out = tmp / "pkg"
    out.mkdir(exist_ok=True)
    return subprocess.run([sys.executable, str(GEN), str(spec), str(out)], capture_output=True, text=True, timeout=60)


def hostile(marker: Path, **over: object) -> dict:
    payload = f"__import__('pathlib').Path({str(marker)!r}).touch()"
    doc = {
        "info": {"title": f"VRX\n{payload}\n#", "version": "1\\"},
        "components": {"schemas": {"RootConfig": {"type": "object", "properties": {
            "x": {"type": "string", "description": f'"""\n{payload}\n"""', "writeOnly": True}}}}},
        "paths": {f'/x"""+str({payload})+"""\\': {"get": {
            "operationId": "Get_x", "summary": f'"""); {payload}; ("""\\',
            "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "title": f"t\n{payload}",
                                                                              "properties": {"a": {"type": "string"}}}}}}}}}},
    }
    doc.update(over)
    return doc


def test_injection_attempts_stay_inert(tmp_path: Path) -> None:
    marker = tmp_path / "pwned"
    r = run_gen(tmp_path, hostile(marker))
    assert r.returncode == 0, r.stderr
    # import the generated package in a fresh interpreter: nothing may run
    code = f"import sys; sys.path.insert(0, {str(tmp_path)!r}); import pkg.models, pkg.operations, pkg.secrets"
    imp = subprocess.run([sys.executable, "-c", code], capture_output=True, text=True, timeout=60)
    assert imp.returncode == 0, imp.stderr
    assert not marker.exists(), "generated code executed an injected payload"
    ops = (tmp_path / "pkg" / "operations.py").read_text()
    assert "def get_x(self)" in ops


def test_hostile_identifiers_fail_generation(tmp_path: Path) -> None:
    for bad in ("x(): pass\nimport os\ndef y", "class", "ünïcode-op", "1abc"):
        doc = hostile(tmp_path / "m")
        path = next(iter(doc["paths"]))
        doc["paths"][path]["get"]["operationId"] = bad
        r = run_gen(tmp_path, doc)
        assert r.returncode != 0 and "not a safe Python identifier" in r.stderr, (bad, r.stderr)
