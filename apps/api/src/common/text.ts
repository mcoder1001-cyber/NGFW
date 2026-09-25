import { isPlainObject, jsonPointer } from '@ngfw/schema';
import { z } from 'zod';
import type { ProblemIssue } from './problem.js';

/**
 * Terminal/log-safe text (D-049, TD-2 #4, P13 review H1). Free text that the API stores or echoes and that a
 * terminal, log line or diff later shows must not carry C0/C1 control characters (ESC starts ANSI/OSC sequences, CR
 * overwrites a line, BEL, NUL …), DEL, or the bidi embedding/override/isolate controls U+202A–U+202E and U+2066–U+2069
 * ("Trojan Source": they reorder what the reader sees), and U+2028/U+2029 (line breaks to JS log viewers and JSON-lines
 * tooling — review L2). TAB is allowed (the schema's single-line rule allows it
 * too); LF only inside configuration documents, where the schema's multi-line fields (banners) need it.
 */
/* eslint-disable no-control-regex -- matching control characters is the purpose of these patterns */
const UNSAFE_SINGLE_LINE =
  /[\u0000-\u0008\u000a-\u001f\u007f-\u009f\u2028\u2029\u202a-\u202e\u2066-\u2069]/u;
const UNSAFE_IN_DOCUMENT =
  /[\u0000-\u0008\u000b-\u001f\u007f-\u009f\u2028\u2029\u202a-\u202e\u2066-\u2069]/u;
/** Same rule as a whole-string pattern: it also goes into the OpenAPI document (`pattern`). */
const SAFE_SINGLE_LINE =
  /^[^\u0000-\u0008\u000a-\u001f\u007f-\u009f\u2028\u2029\u202a-\u202e\u2066-\u2069]*$/u;
/* eslint-enable no-control-regex */

export const UNSAFE_TEXT_MESSAGE =
  'control characters (C0/C1, DEL), line/paragraph separators (U+2028/U+2029) and bidi overrides (U+202A–U+202E, U+2066–U+2069) are not allowed';

/** `U+001B` for the first offending character of `s`, or undefined when `s` is safe. */
export function firstUnsafe(s: string, multiline = false): string | undefined {
  const m = (multiline ? UNSAFE_IN_DOCUMENT : UNSAFE_SINGLE_LINE).exec(s);
  if (m === null) return undefined;
  return `U+${m[0].codePointAt(0)!.toString(16).toUpperCase().padStart(4, '0')}`;
}

/** Shows a string with every unsafe character as `\u{1b}` — for problem messages that must quote user input. */
export function visible(s: string): string {
  return s.replace(
    new RegExp(UNSAFE_IN_DOCUMENT.source + '|\\n', 'gu'),
    (c) => `\\u{${c.codePointAt(0)!.toString(16)}}`,
  );
}

/** Single-line free text of an API request (query, body, path parameter): max length + the safe alphabet. */
export function safeText(max: number) {
  return z.string().max(max).regex(SAFE_SINGLE_LINE, UNSAFE_TEXT_MESSAGE);
}

/**
 * Every string value and object key of a JSON document that carries an unsafe character (LF and TAB allowed), as
 * problem issues with the RFC 6901 pointer of the value, or of the member whose key is unsafe. The configuration
 * schema validates most free text itself (D-049); this is the API's net for the members it does not constrain
 * (descriptions without a pattern, map keys).
 */
export function unsafeTextIssues(doc: unknown): (ProblemIssue & { value?: string })[] {
  const out: (ProblemIssue & { value?: string })[] = [];
  const walk = (node: unknown, path: (string | number)[]): void => {
    if (typeof node === 'string') {
      const c = firstUnsafe(node, true);
      if (c !== undefined)
        out.push({
          pointer: jsonPointer(...path),
          message: `${c}: ${UNSAFE_TEXT_MESSAGE}`,
          value: node,
        });
    } else if (Array.isArray(node)) {
      node.forEach((x, i) => walk(x, [...path, i]));
    } else if (isPlainObject(node)) {
      for (const [k, v] of Object.entries(node)) {
        const c = firstUnsafe(k, false);
        if (c !== undefined) {
          out.push({
            pointer: jsonPointer(...path, visible(k)),
            message: `${c} in a member name: ${UNSAFE_TEXT_MESSAGE}`,
          });
          continue;
        }
        walk(v, [...path, k]);
      }
    }
  };
  walk(doc, []);
  return out.map((i) => ({ ...i, rule: 'api.safe-text' }));
}

/**
 * The unsafe strings `next` has that `base` did not have at the same place (review L2: compared by pointer AND value,
 * so legacy data never lets an edit put a different unsafe value at the same pointer). Issues without the value.
 */
export function newUnsafeTextIssues(base: unknown, next: unknown): ProblemIssue[] {
  const known = new Set(unsafeTextIssues(base).map((i) => `${i.pointer}\u0000${String(i.value)}`));
  return unsafeTextIssues(next)
    .filter((i) => !known.has(`${i.pointer}\u0000${String(i.value)}`))
    .map(({ value: _v, ...i }) => i);
}
