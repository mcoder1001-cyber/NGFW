import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { dataplaneValidators } from './dataplane.js';

const run = (name: string, doc: RootConfigInput) =>
  dataplaneValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

describe('dataplane.workers-match-corelist', () => {
  it('is silent when either side is absent or they agree', () => {
    expect(run('dataplane.workers-match-corelist', {})).toEqual([]);
    expect(run('dataplane.workers-match-corelist', { dataplane: { workers: 2 } })).toEqual([]);
    expect(run('dataplane.workers-match-corelist', { dataplane: { corelist: [2, 3] } })).toEqual([]);
    expect(run('dataplane.workers-match-corelist', { dataplane: { workers: 2, corelist: [2, 3] } })).toEqual([]);
  });
  it('reports a mismatch', () => {
    expect(run('dataplane.workers-match-corelist', { dataplane: { workers: 4, corelist: [2, 3] } })).toEqual([
      { pointer: '/dataplane/workers', message: 'workers (4) must equal the number of cores in corelist (2)' },
    ]);
  });
});

describe('dataplane.corelist-unique', () => {
  it('accepts no corelist or distinct cores; reports repeats', () => {
    expect(run('dataplane.corelist-unique', {})).toEqual([]);
    expect(run('dataplane.corelist-unique', { dataplane: { corelist: [1, 2] } })).toEqual([]);
    expect(run('dataplane.corelist-unique', { dataplane: { corelist: [1, 2, 1] } })).toEqual([
      { pointer: '/dataplane/corelist/2', message: 'core 1 is listed more than once' },
    ]);
  });
});

describe('dataplane.main-core-not-worker', () => {
  it('is silent without mainCore / corelist or when they are disjoint', () => {
    expect(run('dataplane.main-core-not-worker', {})).toEqual([]);
    expect(run('dataplane.main-core-not-worker', { dataplane: { mainCore: 1 } })).toEqual([]);
    expect(run('dataplane.main-core-not-worker', { dataplane: { mainCore: 1, corelist: [2, 3] } })).toEqual([]);
  });
  it('reports a main core that is also a worker core', () => {
    expect(run('dataplane.main-core-not-worker', { dataplane: { mainCore: 2, corelist: [2, 3] } })).toEqual([
      { pointer: '/dataplane/mainCore', message: 'main core 2 is also listed as a worker core' },
    ]);
  });
});

describe('dataplane.pci-unique', () => {
  it('accepts distinct devices and reports repeats case-insensitively', () => {
    expect(run('dataplane.pci-unique', { dataplane: { pciWhitelist: ['0000:0b:00.0', '0000:0b:00.1'] } })).toEqual([]);
    expect(run('dataplane.pci-unique', { dataplane: { pciWhitelist: ['0000:0b:00.0', '0000:0B:00.0'] } })).toEqual([
      { pointer: '/dataplane/pciWhitelist/1', message: 'PCI device 0000:0B:00.0 is listed more than once' },
    ]);
  });
});
