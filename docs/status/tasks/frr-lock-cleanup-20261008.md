# FRR startup error cleanup

Three fixes prevent retained locks, opened-descriptor leaks and unowned cleanup after a contended slot acquisition. Regression cases use temporary filesystem/flock fixtures and do not launch daemons. Startup deadline and process ownership predicate remain unchanged.

Reviewed source local `88d97a9ca9914d673499b275f7c19752ffe648fe`, remote `ef301cbc5b4a9339df626d1a7eadf502e6b8e1ff`, equal tree `e906397df156dc44da74525b3657cbae50b9beb9`; PR209. The WIP contains exact limited-check output and next command. R2/R4 approve source; R1 execution and full hosted gate pending. No source-complete, native acceptance, or original mgmtd failure diagnosis is claimed.

No architecture/contract/security-boundary change and no pending product decision. Remaining work: compile/run all three offline regressions under race tests, complete mandatory quick on final tree, obtain final reviewer/tester evidence, and only then integrate. Live mgmtd/P12 acceptance remains a separate unresolved task.
