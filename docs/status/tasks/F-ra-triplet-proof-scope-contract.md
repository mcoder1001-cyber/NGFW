# Same-boundary proof scope correction

Authorization: parent manager, 2026-10-06. Base local b94e4dbb9c532cf95ea5ae3cfd0dc3bc157469e4, remote 4710aeda82991e9a40f0d0a5cd152be28776301a, tree 49d08f59ae6e278e1e3d0158df410e86e7407298.

Own only namespace_openfile_manager_triplet.go and its tests, plus this contract/WIP/envelope. No other optimization, unit, budget, Source or publisher predicate changes.

Use two explicit private loaders. Source-first loading retains installation proof verification before and after the actual fresh three-unit query. Manager-first loading is reachable only inside numericPublisherManagerUsing's original getter callback: its unchanged original pre-verification and deferred successful post-verification bracket query, parsing and publisher checks. No public skip switch or unchecked boolean.

Role consumption retains explicit caller cancellation, nil/proof identity, complete positive current Source boot identity and one-use checks. It stops repeating installation Verify at role consumption. This retires the newly added guarantee that a direct Source getter rejects installation metadata changed between role consumptions; original Source predicates never supplied that guarantee. Fresh load verification and original manager surrounding verification remain. Tests must prove manager refusal of changed metadata and Source-first query bracketing, cancellation, full Source identity, one-use and foreign refusal.

Successful whole probe/publication removes 30 of 36 additional Verify calls introduced by triplet batching. This is source accounting, not a measured speed or operational readiness claim. Boot32 remains an actual negative. Independent frozen-source review and a new coherent original-unit replay remain mandatory.
