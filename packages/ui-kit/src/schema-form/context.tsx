import { createContext, useContext, type ComponentType } from 'react';
import type { JsonSchema, UiHints } from './types.js';

/** Props handed to a custom widget registered through `<SchemaForm widgets>`. */
export interface WidgetProps {
  schema: JsonSchema;
  /** react-hook-form field path. */
  name: string;
  label: string;
  required: boolean;
  hints: UiHints;
  readOnly: boolean;
  /** Dotted property path without indexes/keys (for label translation), e.g. `subinterfaces.vlanId`. */
  propPath: string;
}

export type WidgetComponent = ComponentType<WidgetProps>;

export interface SchemaFormContextValue {
  root: JsonSchema;
  readOnly: boolean;
  widgets: Readonly<Record<string, WidgetComponent>>;
  /** Choices offered by the `interface-picker` widget (free text is always allowed). */
  interfaceOptions: readonly string[];
  /** Translate a field label; receives the property path and the schema's own title as fallback. */
  translateLabel: (propPath: string, fallback: string) => string;
}

const SchemaFormContext = createContext<SchemaFormContextValue | null>(null);

export const SchemaFormContextProvider = SchemaFormContext.Provider;

export function useSchemaFormContext(): SchemaFormContextValue {
  const ctx = useContext(SchemaFormContext);
  if (!ctx) throw new Error('SchemaForm fields must be rendered inside <SchemaForm>');
  return ctx;
}
