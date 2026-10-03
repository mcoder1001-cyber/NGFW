"""RFC 6901 JSON pointers ↔ `/api/v1/config/{path}` URL paths."""
from __future__ import annotations

from urllib.parse import quote


def escape(segment: str) -> str:
    return segment.replace("~", "~0").replace("/", "~1")


def unescape(segment: str) -> str:
    return segment.replace("~1", "/").replace("~0", "~")


def join(*segments: str | int) -> str:
    """`join("interfaces", "TenGigabitEthernet0/0/0", "mtu")` → `/interfaces/TenGigabitEthernet0~10~10/mtu`."""
    return "".join("/" + escape(str(s)) for s in segments)


def split(pointer: str) -> list[str]:
    if pointer in ("", "/"):
        return []
    if not pointer.startswith("/"):
        pointer = "/" + pointer
    return [unescape(s) for s in pointer[1:].split("/")]


def normalize(pointer: str) -> str:
    """Accept `/interfaces/loop1`, `interfaces/loop1` or `interfaces/loop1/`; return the canonical pointer."""
    p = pointer.strip()
    if p in ("", "/"):
        return ""
    p = "/" + p.strip("/")
    for seg in p[1:].split("/"):
        i = seg.find("~")
        while i >= 0:
            if i + 1 >= len(seg) or seg[i + 1] not in "01":
                raise ValueError(f"invalid JSON pointer escape in {seg!r}")
            i = seg.find("~", i + 2)
    return p


def to_url_path(pointer: str) -> str:
    """Pointer → the `{path}` URL part (no leading slash): each token percent-encoded, `~0`/`~1` escapes kept."""
    return "/".join(quote(seg, safe="~") for seg in normalize(pointer)[1:].split("/")) if normalize(pointer) else ""


def matches(pattern: str, pointer: str) -> bool:
    """`/management/users/*/passwordHash` matches `/management/users/0/passwordHash` (`*` = one token)."""
    ps, xs = pattern.split("/"), pointer.split("/")
    return len(ps) == len(xs) and all(p in ("*", x) for p, x in zip(ps, xs))
