# S-cli-ipsec — native state and runtime action acceptance (2026-10-03)

Implemented in the shared workspace and ready for review. Commands use generated REST operations only:

```text
show ipsec tunnels [<tunnel>]
show ipsec sa [<tunnel>] [limit <1..1000>] [offset <0..1000000>]
ipsec initiate <tunnel>
ipsec rekey <tunnel> <child-spi>
ipsec delete-sa <tunnel> <ike-spi>
```

State commands preserve the API JSON in machine mode and format it in interactive output. Invalid or duplicate paging, zero/negative/overflow SPIs and incorrect arguments are refused before a network request. SPI parsing supports decimal and `0x` hexadecimal without silently treating leading-zero decimal as octal. IKE SPI values remain decimal strings through JSON, preserving the full unsigned 64-bit range.

HTTP acceptance tests verify tunnel filtering/paging, REST-only state reads, exact JSON/counter output, no network side effect for invalid input, administrator refusal (403), native plugin unavailability (503), leading-zero decimal interpretation and 64-bit SPI precision. The generated CLI reference was refreshed.

Validation: `tools/heavy.sh make -C apps/cli docs test build`, followed by the corrected full test/build run, passed. The suite used race detection across all CLI packages; the binary build completed. Evidence: [full CLI race tests and build](S-cli-ipsec-2026-10-03-evidence/cli-race-build.log). Task status is `review`; no merge or deployment is claimed. No direct agent/VPP access or host changes were introduced.
