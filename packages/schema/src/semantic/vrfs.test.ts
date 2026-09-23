import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { vrfsValidators } from './vrfs.js';

const run = (name: string, doc: RootConfigInput) =>
  vrfsValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

describe('vrfs.id-unique', () => {
  it('accepts distinct ids and reports every later duplicate', () => {
    expect(run('vrfs.id-unique', { vrfs: { a: { id: 1 }, b: { id: 2 } } })).toEqual([]);
    expect(run('vrfs.id-unique', { vrfs: { a: { id: 1 }, b: { id: 1 }, c: { id: 1 } } })).toEqual([
      { pointer: '/vrfs/b/id', message: "table id 1 is already used by VRF 'a'" },
      { pointer: '/vrfs/c/id', message: "table id 1 is already used by VRF 'a'" },
    ]);
  });
});

describe('vrfs.default-is-table-zero', () => {
  it('lets default be declared with id 0 and others with any other id', () => {
    expect(run('vrfs.default-is-table-zero', { vrfs: { default: { id: 0 }, a: { id: 10 } } })).toEqual([]);
    expect(run('vrfs.default-is-table-zero', {})).toEqual([]);
  });
  it('rejects default with a non-zero id and any other VRF with id 0', () => {
    expect(run('vrfs.default-is-table-zero', { vrfs: { default: { id: 5 }, a: { id: 0 } } })).toEqual([
      { pointer: '/vrfs/default/id', message: "the 'default' VRF is VPP table 0 and cannot use id 5" },
      { pointer: '/vrfs/a/id', message: "table id 0 is the 'default' VRF; choose another id for 'a'" },
    ]);
  });
});
