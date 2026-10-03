# PKI export query source re-review — 2026-10-03

Reviewed immutable98ca515809a0acccdb6082eaae2e2bef5ec3b7a0 in PKI-merge-ready. Source repair APPROVE; generated selector checkpoint and validation remain pending manager handoff.

ApiQuery explicitly declares optional kind and derives its schema from the actual ExportQuery.shape.kind, preserving certificate default and certificate/ca/crl/key enum without duplicated contract values. Public CLI URL regression checks all three successful public selectors against complete expected URLs and checks that undocumented force is still refused. SDK compile consumer requests ca and expects an invalid enum diagnostic on the query member. The existing service rejects key with403 before reading documents or secrets and filters output to public certificate/CRL PEM blocks; metadata disclosure of the forbidden key selector does not grant access.

No source or product edits, test/build/generator/heavy commands executed by reviewer. Root is regenerating through actual combined OpenAPI and CLI generators; must verify generated CLI QueryParams contains kind and generated TS optional query kind bounded enum, then run finite regression/typecheck and full hosted gate. This source approval does not assert generated output or those checks are complete. Whole agent PKI runtime remains incomplete.

## Final generated checkpoint verification

Independently inspected committed3bcf981b61c3ae179f4590ad2ca945be4e01e715 in PKI-merge-ready. Final verdict APPROVE source and generated contract repair, with root-owned execution limits. Generated CLI Pki_export now declares QueryParams:[kind], preserving its GET/path/name/body metadata. Generated SDK declares optional query and optional kind with exact certificate|ca|crl|key union; omitted selector corresponds to the unchanged Zod certificate default. Default values are applied at the server boundary rather than by TypeScript types. Actual enum remains bounded; no widening or arbitrary-query acceptance is introduced. Existing service key403 early rejection and public PEM filter remain unchanged. Earlier P2 in05b92b8c is resolved by98ca5158 plus3bcf981b.

Root reports all13 actual generator tasks and CLI generation completed; reviewer inspected committed output only. Root finite CLI lint/race/build, SDK consumer compile and scoped lint/check jobefd64d39 remains manager-owned. No test pass is claimed by this reviewer, and whole agent PKI runtime is still outside this checkpoint.
