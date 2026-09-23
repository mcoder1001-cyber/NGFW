/**
 * JSON Schema 2020-12 subset understood by `<SchemaForm>`. Unknown keywords are kept (and ignored) so the
 * generated schemas from `packages/schema` (`z.toJSONSchema`) can be passed through untouched.
 */
export type JsonSchemaType = 'string' | 'number' | 'integer' | 'boolean' | 'object' | 'array' | 'null';

export interface JsonSchema {
  $schema?: string;
  $id?: string;
  $ref?: string;
  $defs?: Record<string, JsonSchema>;
  type?: JsonSchemaType | JsonSchemaType[];
  title?: string;
  description?: string;
  default?: unknown;
  const?: unknown;
  enum?: unknown[];
  properties?: Record<string, JsonSchema>;
  required?: string[];
  additionalProperties?: JsonSchema | boolean;
  propertyNames?: JsonSchema;
  items?: JsonSchema;
  prefixItems?: JsonSchema[];
  minItems?: number;
  maxItems?: number;
  uniqueItems?: boolean;
  minLength?: number;
  maxLength?: number;
  pattern?: string;
  format?: string;
  minimum?: number;
  maximum?: number;
  exclusiveMinimum?: number;
  exclusiveMaximum?: number;
  multipleOf?: number;
  oneOf?: JsonSchema[];
  anyOf?: JsonSchema[];
  allOf?: JsonSchema[];
  readOnly?: boolean;
  writeOnly?: boolean;
  deprecated?: boolean;
  'x-vrx-ui'?: UiHints;
  [keyword: string]: unknown;
}

/** Name of the JSON Schema extension keyword carrying form-renderer hints (packages/schema `withUi()`, D-019). */
export const X_VRX_UI = 'x-vrx-ui' as const;

/** `dependsOn`: show the field only while a sibling (or absolute `/path`) has a matching value. */
export interface UiDependsOn {
  /** Sibling property name, or an absolute JSON path starting with `/`. */
  field: string;
  /** Exact value required; omit `value`/`values` to require any truthy value. */
  value?: unknown;
  /** Any of these values. */
  values?: unknown[];
}

/**
 * Hints emitted by `withUi()` in packages/schema. `widget` names understood here:
 * text · textarea · password · select · radio · number · slider · switch · checkbox · cidr · ip · mac ·
 * interface-picker · chips · multiselect · list · json · hidden. Unknown widgets fall back to the type default.
 */
export interface UiHints {
  widget?: string;
  group?: string;
  order?: number;
  /** Help text, or an i18n key (resolved when it exists in the app's resources). */
  help?: string;
  dependsOn?: string | UiDependsOn;
  placeholder?: string;
  [hint: string]: unknown;
}

/** RFC 9457 problem details as vrx-api returns them; `pointer` is the RFC 6901 path of the offending value. */
export interface ProblemDetails {
  type?: string;
  title?: string;
  status?: number;
  detail?: string;
  instance?: string;
  pointer?: string;
  /** Several field errors at once (validation reports). */
  errors?: ProblemFieldError[];
  [extension: string]: unknown;
}

export interface ProblemFieldError {
  pointer: string;
  detail?: string;
  title?: string;
}

/** A path into a JSON document; record keys are plain segments (not escaped). */
export type JsonPath = readonly (string | number)[];

/** Translation function shape used by the renderer (the `ui-kit` namespace). */
export type Translate = (key: string, options?: Record<string, unknown>) => string;
