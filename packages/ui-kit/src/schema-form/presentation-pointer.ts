import { productWording } from '../i18n/product-wording.js';
import { mergeAllOf, parsePointer, resolveRef, titleOf, variantsOf } from './schema-utils.js';
import type { JsonSchema } from './types.js';

/** Label schema-owned locations in diagnostic text without changing error-mapping pointers or user record keys. */
export function presentationPointer(root: JsonSchema, pointer: string, language: string): string {
  let node: JsonSchema = root;
  const labels = parsePointer(pointer).map((part) => {
    const resolved = mergeAllOf(resolveRef(node, root), root);
    const alternatives = [
      resolved,
      ...(variantsOf(resolved) ?? []).map((s) => resolveRef(s, root)),
    ];
    const property = alternatives.map((s) => s.properties?.[part]).find((s) => s !== undefined);
    if (property) {
      node = property;
      return /frr|vpp|strongswan/i.test(part)
        ? productWording(titleOf(property, part), language)
        : part;
    }
    const collection = alternatives.find(
      (s) => s.items !== undefined || typeof s.additionalProperties === 'object',
    );
    node =
      collection?.items ??
      (typeof collection?.additionalProperties === 'object' ? collection.additionalProperties : {});
    return part;
  });
  return labels.length === 0
    ? pointer
    : `/${labels.map((s) => s.replace(/~/g, '~0').replace(/\//g, '~1')).join('/')}`;
}
