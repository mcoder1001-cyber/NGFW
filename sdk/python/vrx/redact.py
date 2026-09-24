"""Client-side redaction of write-only leaves (D-046) — the same rule the API applies to everything it returns.
The pointer patterns are generated from the OpenAPI document (`writeOnly: true`), so new secret fields need no code."""
from __future__ import annotations

import copy
from typing import Any

from ._generated.secrets import WRITE_ONLY_POINTERS
from .pointer import escape, matches, normalize

REDACTED = "<redacted>"


def secret_pointers(value: Any, at: str = "") -> list[str]:
    """Pointers of the write-only leaves present in `value`, which sits at pointer `at` of the document."""
    base = normalize(at)
    found: list[str] = []

    def walk(v: Any, ptr: str) -> None:
        if any(matches(p, ptr) for p in WRITE_ONLY_POINTERS):
            found.append(ptr)
            return
        if isinstance(v, dict):
            for k, child in v.items():
                walk(child, f"{ptr}/{escape(str(k))}")
        elif isinstance(v, list):
            for i, child in enumerate(v):
                walk(child, f"{ptr}/{i}")

    walk(value, base)
    return found


def redact(value: Any, at: str = "") -> Any:
    """A deep copy of `value` (located at pointer `at`) with every write-only leaf replaced by `<redacted>`."""
    base = normalize(at)
    ptrs = secret_pointers(value, base)
    if not ptrs:
        return value
    out = copy.deepcopy(value)
    for ptr in ptrs:
        tokens = ptr[len(base) + 1 :].split("/") if len(ptr) > len(base) else []
        if not tokens:
            return REDACTED
        node = out
        for t in tokens[:-1]:
            node = node[int(t)] if isinstance(node, list) else node[t.replace("~1", "/").replace("~0", "~")]
        last = tokens[-1]
        if isinstance(node, list):
            node[int(last)] = REDACTED
        else:
            node[last.replace("~1", "/").replace("~0", "~")] = REDACTED
    return out
