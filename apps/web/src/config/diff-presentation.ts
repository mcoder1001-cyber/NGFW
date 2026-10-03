import { isPlainObject, jsonPointer, parsePointer } from '@ngfw/schema';
import { productEngineLabels } from '@ngfw/ui-kit';
import { rootSchema } from '../schema/registry';
import { schemaAt } from './refine';

/** Display labels for schema-owned members, never for arbitrary record keys. */
function memberLabel(path: readonly string[]): string {
  const name = path.at(-1) ?? '';
  if (path.length === 3 && path[0] === 'services' && path[1] === 'dns' && name === 'vppCache') {
    return 'dataplaneCache';
  }
  if (
    path.length === 4 &&
    path[0] === 'routing' &&
    path[1] === 'static' &&
    /^\d+$/.test(path[2]!) &&
    name === 'viaFrr'
  ) {
    return 'viaRoutingService';
  }
  return name;
}

/** A presentation path only; API pointers and change identities remain untouched. */
export function diffPointerLabel(pointer: string): string {
  try {
    const path = parsePointer(pointer);
    return jsonPointer(...path.map((_part, i) => memberLabel(path.slice(0, i + 1))));
  } catch {
    return pointer;
  }
}

/** Only actual engine enums may receive implementation-independent value labels. */
function scalarLabel(value: unknown, path: readonly string[], language: string): unknown {
  if (typeof value !== 'string' || path.at(-1) !== 'engine') return value;
  // A historical revision may contain the retired engine. Label it for display
  // without accepting it in current configuration or changing arbitrary strings.
  if (value === 'strongswan' && path.length === 5 && path[0] === 'vpn' &&
      path[1] === 'ipsec' && path[2] === 'tunnels') {
    return productEngineLabels(language)[value] ?? value;
  }
  const schema = schemaAt(rootSchema, jsonPointer(...path));
  return Array.isArray(schema?.enum) && schema.enum.includes(value)
    ? (productEngineLabels(language)[value] ?? value)
    : value;
}

/**
 * JSON-shaped display text, not a configuration document. Walk the original schema path so labels only replace
 * known metadata and enum choices, including inside whole-domain changes. Record keys, descriptions, interface
 * names and unknown values retain their exact spelling. Serializing entries directly also avoids losing a value
 * if a future/unknown member happens to have the same spelling as a display label.
 */
export function diffValueText(
  value: unknown,
  pointer: string,
  language = 'en',
): string | undefined {
  let path: string[];
  try {
    path = parsePointer(pointer);
  } catch {
    return JSON.stringify(value, null, 2);
  }
  function render(current: unknown, at: readonly string[], depth: number): string | undefined {
    const indent = '  '.repeat(depth);
    const childIndent = `${indent}  `;
    if (Array.isArray(current)) {
      const items = current.map(
        (item, i) => `${childIndent}${render(item, [...at, String(i)], depth + 1) ?? 'null'}`,
      );
      return items.length ? `[\n${items.join(',\n')}\n${indent}]` : '[]';
    }
    if (isPlainObject(current)) {
      const members = Object.entries(current).flatMap(([key, item]) => {
        const childPath = [...at, key];
        const text = render(item, childPath, depth + 1);
        return text === undefined
          ? []
          : [`${childIndent}${JSON.stringify(memberLabel(childPath))}: ${text}`];
      });
      return members.length ? `{\n${members.join(',\n')}\n${indent}}` : '{}';
    }
    return JSON.stringify(scalarLabel(current, at, language));
  }
  return render(value, path, 0);
}
