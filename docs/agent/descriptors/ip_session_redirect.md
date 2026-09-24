# ip_session_redirect descriptors (DF-2, WBS D2.7)

Package `apps/agent/internal/descriptors/ip_session_redirect` (Go package `sessionredirect`). Messages from `apps/agent/binapi/ip_session_redirect`.

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| session redirect | `ip-session-redirect.redirect` / `ip-session-redirect.redirect/<table>/<hex(match)>` | `ip_session_redirect_add_v2` (table index, match, opaque, is_punt, af, paths) / `ip_session_redirect_del`; Update `ErrRecreate` (re-adding with other contents returns -52 on vrx-a) | `ip_session_redirect_dump` per owned classify table (match, opaque, punt, af, paths decoded) | `classify.table/<table>`, path interfaces (Optional) | Table name → index via the classify Store; `Normalize` canonicalises match and paths. A dump exists in 26.06, so this descriptor is complete (not partial). |

CLI: `show ip session redirect`.
