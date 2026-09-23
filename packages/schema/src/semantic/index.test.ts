import { describe, expect, it } from 'vitest';
import { RootConfig, ROOT_KEYS } from '../index.js';
import { SEMANTIC_VALIDATORS, semanticRegistry, validateSemantics } from './index.js';
import { SemanticRegistry, sortIssues, type ValidatorDefinition } from './registry.js';

describe('SemanticRegistry', () => {
  const noAdmin: ValidatorDefinition = {
    name: 'management.admin-exists',
    domains: ['management'],
    validate: () => [
      { pointer: '/management/users', message: 'at least one admin user is required' },
    ],
  };
  const vrfExists: ValidatorDefinition = {
    name: 'interfaces.vrf-exists',
    domains: ['interfaces', 'vrfs'],
    validate: () => [{ pointer: '/interfaces/Gig0~10~10/vrf', message: "VRF 'x' does not exist" }],
  };

  it('registers validators, lists them in order and rejects duplicate names', () => {
    const r = new SemanticRegistry().register(noAdmin).register(vrfExists);
    expect(r.list().map((v) => v.name)).toEqual([
      'management.admin-exists',
      'interfaces.vrf-exists',
    ]);
    expect(r.has('interfaces.vrf-exists')).toBe(true);
    expect(() => r.register({ ...noAdmin })).toThrow(/already registered/);
  });

  it('runs every validator and sorts issues by pointer then message', () => {
    const r = new SemanticRegistry().register(noAdmin).register(vrfExists);
    expect(r.run(RootConfig.parse({}))).toEqual([
      { pointer: '/interfaces/Gig0~10~10/vrf', message: "VRF 'x' does not exist" },
      { pointer: '/management/users', message: 'at least one admin user is required' },
    ]);
  });

  it('can be restricted to validators that read the given domains', () => {
    const r = new SemanticRegistry().register(noAdmin).register(vrfExists);
    expect(r.run(RootConfig.parse({}), ['vrfs']).map((i) => i.pointer)).toEqual([
      '/interfaces/Gig0~10~10/vrf',
    ]);
    expect(r.run(RootConfig.parse({}), ['nat'])).toEqual([]);
  });

  it('sortIssues is stable and pure', () => {
    const issues = [
      { pointer: '/b', message: 'y' },
      { pointer: '/a', message: 'z' },
      { pointer: '/a', message: 'x' },
    ];
    expect(sortIssues(issues)).toEqual([
      { pointer: '/a', message: 'x' },
      { pointer: '/a', message: 'z' },
      { pointer: '/b', message: 'y' },
    ]);
    expect(issues[0]).toEqual({ pointer: '/b', message: 'y' });
  });
});

describe('validateSemantics (process-wide registry)', () => {
  it('has unique validator names, each prefixed with an existing root key', () => {
    const names = SEMANTIC_VALIDATORS.map((v) => v.name);
    expect(new Set(names).size).toBe(names.length);
    for (const v of SEMANTIC_VALIDATORS) {
      const prefix = v.name.split('.')[0];
      expect(ROOT_KEYS).toContain(prefix);
      expect(v.domains).toContain(prefix);
    }
    expect(semanticRegistry.list()).toHaveLength(SEMANTIC_VALIDATORS.length);
  });

  it('reports only the bootstrap issue (no admin user) for the empty document', () => {
    expect(validateSemantics(RootConfig.parse({}))).toEqual([
      {
        pointer: '/management/users',
        message: 'at least one enabled admin user with a password or an SSH key is required',
      },
    ]);
  });

  it('reports no issues for the minimal committable document', () => {
    const minimal = RootConfig.parse({
      management: { users: [{ username: 'admin', role: 'admin', passwordHash: '$vrx-test$VRX_TEST_HASH_admin' }] },
    });
    expect(validateSemantics(minimal)).toEqual([]);
    expect(validateSemantics(minimal, ['interfaces'])).toEqual([]);
  });
});
