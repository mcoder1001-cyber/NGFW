import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { directionFor, UI_KIT_NS } from '../../i18n/index.js';

/**
 * Help and error texts are prose in the UI language, so their direction is the active locale's — set explicitly, never
 * guessed from the first strong character: Persian help that starts with a Latin acronym ("MTU لایه‌ی ۳ …",
 * "polling (…)، interrupt یا adaptive") must stay RTL (review M2). `dir` on the span also isolates it (HTML's
 * `[dir] { unicode-bidi: isolate }`). Only identifiers get an explicit LTR `<bdi>` (chips, identifier cells/summaries).
 */
function Prose({ text }: { text: string }) {
  const { i18n } = useTranslation(UI_KIT_NS);
  return <span dir={directionFor(i18n.resolvedLanguage ?? i18n.language)}>{text}</span>;
}

export function prose(text: string | undefined): ReactNode {
  return text === undefined || text === '' ? undefined : <Prose text={text} />;
}

/** An identifier (prefix, name, address) inside prose or a table: isolated and always LTR (RTL-1). */
export function identifier(text: string): ReactNode {
  return text === '' ? text : <bdi dir="ltr">{text}</bdi>;
}
