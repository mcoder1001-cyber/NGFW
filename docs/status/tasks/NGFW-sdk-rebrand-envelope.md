# NGFW SDK rebrand envelope

Own branch codex/ngfw-rebrand-sdk-20261003 in work/NGFW-rebrand-sdk; base07aa93cbaeb764484bb2dc0ffe52bd43fcff4cb0 with verified renamed application/contracts.
Root transferred sdk/** Python/Terraform/generator/config/tests ownership only, plus unique NGFW-sdk-rebrand status docs. No app/root/otherworktree edits. Rename all technical/productNGFW names/paths consistently, canonical generate from actual renamed OpenAPI; preserve field semantics, dependency versions and lock integrity.
No package installs, external infrastructure/product calls, host services/migration, auth/model override or self-review. Existing SDK unit tests use fake HTTP/protocol harness; live tests remain explicit NOTRUN without configured appliance. Reuse existing pinned test tooling if present, otherwise record unavailable tooling truthfully. Root publishes each coherent checkpoint and owns final hosted/fullintegration/independentreview.
