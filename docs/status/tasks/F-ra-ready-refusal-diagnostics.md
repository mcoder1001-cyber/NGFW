# READY refusal diagnostics

Boot23 actual console evidence passed peer/reference and refused in the old source-image region, which also included held installation-proof and manager checks. This change records the existing SourceImage, Proof and Manager stage immediately before each unchanged predicate in the same order. It preserves the established log prefix and adds only an inner numeric stage 1..24 (or 0 for unclassified); no raw error or caller values. Manager errors retain ErrBoundary classification. No authorization, capability, unit, deadline or path changes.

The first local run overlapped a source edit and is not acceptance; it exposed a log-prefix regression. After preserving that prefix, the unchanged NumericPublisher race suite passed 1.287s, actual EXIT0. Actual guest replay and independent review pending.
