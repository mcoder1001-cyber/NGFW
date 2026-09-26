/**
 * F-object-model web exports for the consumers (F-acl, F-host-acl-nftables): the object/tag picker widgets for
 * `<SchemaForm widgets={objectModelWidgets}>` and the helpers they need. Stable; see docs/agent/objects.md.
 */
export { ObjectPicker, TagPicker, objectModelWidgets } from './ObjectPicker';
export { OBJECT_KINDS, PICKER_KINDS, pickerKinds, summary, type ObjectKind } from './model';
export { useCandidateObjects, useFqdnState, useUsage } from './queries';
export { TagChips, UsageDrawer } from './parts';
