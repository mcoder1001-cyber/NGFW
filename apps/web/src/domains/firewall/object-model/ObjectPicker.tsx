import { SchemaField, type JsonSchema, type WidgetComponent, type WidgetProps } from '@ngfw/ui-kit/schema-form';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { entriesOf, pickerKinds, summary, type ObjectKind, type ObjectsConfig } from './model';
import { useCandidateObjects } from './queries';

/**
 * The `object-picker` / `tag-picker` custom widget of `<SchemaForm widgets>` (P08 pattern; not a ui-kit built-in).
 * It offers the names of the candidate's objects of the kinds the field may reference — `pickerKinds()`: an explicit
 * `x-vrx-ui.objectKinds`, the kinds its schema help names (`objects.addresses or objects.addressGroups`), or its
 * property name (`zone`, `schedule`, `tags`) — labelled with their kind and value. A string field becomes a select, an
 * array (group members, tags) a multi-select; validation stays the form's (the schema of the whole document).
 *
 * Consumers (F-acl, F-host-acl-nftables) pass `objectModelWidgets` as `<SchemaForm widgets={objectModelWidgets}>`;
 * the picker loads the candidate's objects itself (TanStack Query, key `['config', 'candidate', 'objects']`).
 */
export const ObjectPicker: WidgetComponent = (props: WidgetProps) => {
  const { t } = useTranslation('object-model');
  const objects = useCandidateObjects();
  const kinds = pickerKinds(props.hints, props.propPath);
  const { schema, hints } = useMemo(() => pickerSchema(props, kinds, objects.data, (k, o) => t(k, o ?? {})), [props, kinds, objects.data, t]);
  const parentName = props.name.includes('.') ? props.name.slice(0, props.name.lastIndexOf('.')) : props.name;
  return (
    <SchemaField
      schema={{ ...schema, 'x-vrx-ui': hints }}
      name={props.name}
      propPath={props.propPath}
      parentName={parentName}
      label={props.label}
      required={props.required}
    />
  );
};

/** Tags use the same picker (kind `tags`). */
export const TagPicker: WidgetComponent = ObjectPicker;

/** `<SchemaForm widgets={objectModelWidgets}>`: the object and tag pickers. */
export const objectModelWidgets: Readonly<Record<string, WidgetComponent>> = {
  'object-picker': ObjectPicker,
  'tag-picker': TagPicker,
};

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/** The field's schema with the choices as an `enum` (+ labels) and a select/multiselect widget. */
export function pickerSchema(
  props: Pick<WidgetProps, 'schema' | 'hints'>,
  kinds: readonly ObjectKind[],
  objects: Partial<ObjectsConfig> | undefined,
  t: Translate,
): { schema: JsonSchema; hints: Record<string, unknown> } {
  const choices: string[] = [];
  const labels: Record<string, string> = {};
  for (const kind of kinds) {
    for (const [name, entry] of entriesOf(objects, kind)) {
      if (labels[name] !== undefined) continue;
      choices.push(name);
      const detail = summary(kind, entry);
      labels[name] = t('picker.option', { name, kind: t(`kindOne.${kind}`), detail: detail || '—' });
    }
  }
  // dependsOn was applied by SchemaForm around this widget already; the widget swap must not recurse into itself
  const rest: Record<string, unknown> = { ...props.hints };
  delete rest.dependsOn;
  delete rest.widget;
  const isArray = props.schema.type === 'array';
  const hints = { ...rest, widget: isArray ? 'multiselect' : 'select', enumLabels: labels, objectKinds: [...kinds] };
  const schema: JsonSchema = isArray
    ? { ...props.schema, items: { ...(props.schema.items ?? {}), enum: choices }, uniqueItems: true }
    : { ...props.schema, enum: choices };
  return { schema, hints };
}
