import type { z } from 'zod';
import { formatForPattern } from './form-value.js';
import type { Translate } from './types.js';

type RawIssue = z.core.$ZodRawIssue;

const FORMAT_KEYS: Record<string, string> = {
  ipv4: 'validation.ipv4',
  ipv6: 'validation.ipv6',
  cidrv4: 'validation.cidrv4',
  cidrv6: 'validation.cidrv6',
  email: 'validation.email',
  url: 'validation.uri',
  uri: 'validation.uri',
  hostname: 'validation.hostname',
  mac: 'validation.mac',
  regex: 'validation.pattern',
};

/** One Zod 4 issue → translated message from the `ui-kit` namespace (`validation.*`). */
export function issueMessage(issue: RawIssue, t: Translate): string {
  switch (issue.code) {
    case 'invalid_type': {
      if (issue.input === undefined || issue.input === null || issue.input === '') return t('validation.required');
      if (issue.expected === 'int') return t('validation.integer');
      if (issue.expected === 'number') return t('validation.number');
      return t('validation.invalidType');
    }
    case 'too_small': {
      const limit = Number(issue.minimum);
      const inclusive = issue.inclusive !== false;
      if (issue.origin === 'string') {
        if (limit <= 1 && (issue.input === '' || issue.input === undefined)) return t('validation.required');
        return t('validation.minLength', { count: limit });
      }
      if (issue.origin === 'array' || issue.origin === 'set') return t('validation.minItems', { count: limit });
      return t(inclusive ? 'validation.minimum' : 'validation.exclusiveMinimum', { limit });
    }
    case 'too_big': {
      const limit = Number(issue.maximum);
      const inclusive = issue.inclusive !== false;
      if (issue.origin === 'string') return t('validation.maxLength', { count: limit });
      if (issue.origin === 'array' || issue.origin === 'set') return t('validation.maxItems', { count: limit });
      return t(inclusive ? 'validation.maximum' : 'validation.exclusiveMaximum', { limit });
    }
    case 'invalid_format': {
      const format = issue.format === 'regex' ? (formatForPattern(issue.pattern) ?? 'regex') : issue.format;
      return t(FORMAT_KEYS[format] ?? 'validation.pattern');
    }
    case 'not_multiple_of':
      return t('validation.multipleOf', { step: Number(issue.divisor) });
    case 'invalid_value':
      return t('validation.enum');
    case 'invalid_union':
      return t('validation.union');
    case 'unrecognized_keys':
      return `${t('validation.unknownKey')}: ${issue.keys.join(', ')}`;
    case 'invalid_key':
    case 'invalid_element':
      return t('validation.invalidType');
    case 'custom':
      return issue.message ?? t('validation.invalidType');
    default:
      return t('validation.invalidType');
  }
}
