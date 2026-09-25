package ipfix

// TD-11b declarations (dfkit/persist): the product agent refuses to register a descriptor that does
// not say how it records ownership (gap found by F-ipfix-sflow: subsystems'
// TestRequirePersistentPerFamily failed once the family was registered). None of these records
// ownership in a claim or boot store:

// RecordsNoOwnership declares: exporter 0 is a VPP-global singleton (D-071 role, no records).
func (*DefaultExporterDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares: additional exporters are ours by collector address scope (WithCollectorScope).
func (*ExporterDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares: the classify stream is a VPP-global singleton (D-071 role, no records).
func (*ClassifyStreamDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares: classify tables are ours through DF-2's classify store, which this
// descriptor only reads (DF-2's own descriptors declare that store).
func (*ClassifyTableDescriptor) RecordsNoOwnership() {}
