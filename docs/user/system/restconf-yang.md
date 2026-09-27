# RESTCONF and YANG

The box exposes a **RESTCONF** API (RFC 8040) at `/restconf`, described by **YANG 1.1** modules that are *generated*
from the same configuration schema the REST API and the web UI use. There is one module per configuration domain
(`vrx-system`, `vrx-interfaces`, `vrx-routing`, …). RESTCONF is a thin compatibility layer over the normal
candidate/commit engine: it uses the same authentication, roles and audit as `/api/v1`, and a write is not live until
you commit.

**System → RESTCONF / YANG** lists the modules and lets you download them for your NETCONF/RESTCONF tooling.

## Authentication

Same as the REST API — send your bearer token (or API key):

```
curl -H "Authorization: Bearer $TOKEN" -H "Accept: application/yang-data+json" \
     https://<box>/restconf/data/vrx-interfaces:interfaces
```

## Reading configuration

`GET /restconf/data` returns the whole datastore; `GET /restconf/data/<module>:<node>[/…]` returns one subtree. The
top node is module-qualified (`{"vrx-system:system": {…}}`). Reads come from the **running** datastore by default; add
`?datastore=candidate` to read the uncommitted candidate. Secret leaves (e.g. password hashes) are **never** returned.

```
# the whole running config
curl … https://<box>/restconf/data
# one domain
curl … https://<box>/restconf/data/vrx-system:system
# a keyed list entry (RFC 8040 list-key syntax)
curl … https://<box>/restconf/data/vrx-interfaces:interfaces=TenGigabitEthernet0/mtu
```

## Changing configuration

`PUT` replaces, `PATCH` merges (RFC 7386), `DELETE` removes — all edit the **candidate**. Then run the `vrx:commit`
operation. The body is module-qualified `application/yang-data+json`:

```
# stage a change
curl -X PATCH -H "Content-Type: application/yang-data+json" … \
     -d '{"vrx-system:system":{"hostname":"edge-1"}}' \
     https://<box>/restconf/data/vrx-system:system

# apply it (optionally { "input": { "confirm": 120 } } for a confirmed commit)
curl -X POST -H "Content-Type: application/yang-data+json" … \
     -d '{"input":{}}' https://<box>/restconf/operations/vrx:commit
```

Other operations: `POST /restconf/operations/vrx:confirm` confirms a pending confirmed commit, and
`POST /restconf/operations/vrx:rollback` with `{"input":{"revision":<n>}}` restores an earlier revision.

## Errors

Errors use the RFC 8040 `ietf-restconf:errors` body; the offending JSON pointer is in `error-path`:

```json
{ "ietf-restconf:errors": { "error": [
  { "error-type": "application", "error-tag": "invalid-value",
    "error-path": "/system/hostname", "error-message": "…" } ] } }
```

Authentication and authorization failures (401/403) stay `application/problem+json`, the same as the REST API.

## Discovery

- `GET /.well-known/host-meta` — points a client at `/restconf`.
- `GET /restconf` — the API resource.
- `GET /restconf/data/ietf-yang-library:yang-library` — the module set (RFC 8525).

## Known deviations

- RESTCONF is **not** in the OpenAPI document, so the generated REST client, the CLI and the SDK do not carry generic
  RESTCONF operations.
- YANG is generated for **configuration** only; operational state (`config false`) beyond yang-library is not modelled.
- A schema list with no natural key is emitted as a keyless (`ordered-by user`) list — a documented deviation, since a
  YANG config list normally needs a key.
- Regex patterns that cannot be expressed in XSD (lookahead, word boundaries) are dropped with a comment in the module.
- Validation with `pyang`/`yanglint` is a manual gate — neither tool is installed in the build image.

## Out of scope

NETCONF (SSH/XML), RESTCONF event streams (SSE), and gNMI are not provided.
