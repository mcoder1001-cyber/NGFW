import { describe, expect, it } from 'vitest';
import { formatTagStack, liveTagStack, sameTagStack, tagStack } from './tagStack';

describe('tag stack formatter (F-vlan-qinq)', () => {
  it('dot1q single tag, dot1ad + dot1q QinQ, dot1q-in-dot1q, dot1ad single tag', () => {
    expect(formatTagStack(tagStack({ vlanId: 100 }))).toBe('dot1q 100');
    expect(formatTagStack(tagStack({ vlanId: 100, dot1ad: false }))).toBe('dot1q 100');
    expect(formatTagStack(tagStack({ vlanId: 200, innerVlanId: 100, dot1ad: true }))).toBe(
      'dot1ad 200 · dot1q 100',
    );
    expect(formatTagStack(tagStack({ vlanId: 300, innerVlanId: 30 }))).toBe('dot1q 300 · dot1q 30');
    expect(formatTagStack(tagStack({ vlanId: 201, dot1ad: true }))).toBe('dot1ad 201');
  });

  it('outermost tag first; the inner tag is always 802.1Q', () => {
    expect(tagStack({ vlanId: 200, innerVlanId: 100, dot1ad: true })).toEqual([
      { proto: 'dot1ad', vlan: 200 },
      { proto: 'dot1q', vlan: 100 },
    ]);
  });

  it('no outer tag is no stack; 0 is "no tag" (live state), null/undefined are absent', () => {
    expect(tagStack(undefined)).toEqual([]);
    expect(tagStack({})).toEqual([]);
    expect(tagStack({ vlanId: 0, innerVlanId: 100 })).toEqual([]);
    expect(formatTagStack(tagStack({ vlanId: 100, innerVlanId: 0 }))).toBe('dot1q 100');
    expect(formatTagStack(tagStack({ vlanId: 100, innerVlanId: null, dot1ad: null }))).toBe(
      'dot1q 100',
    );
    expect(formatTagStack([])).toBe('');
  });

  it('label and separator are replaceable', () => {
    const s = tagStack({ vlanId: 200, innerVlanId: 100, dot1ad: true });
    expect(formatTagStack(s, (t) => `${t.proto.toUpperCase()}:${t.vlan}`, ' / ')).toBe(
      'DOT1AD:200 / DOT1Q:100',
    );
  });

  it('sameTagStack compares type and numbers of every tag', () => {
    const a = tagStack({ vlanId: 200, innerVlanId: 100, dot1ad: true });
    expect(sameTagStack(a, tagStack({ vlanId: 200, innerVlanId: 100, dot1ad: true }))).toBe(true);
    expect(sameTagStack(a, tagStack({ vlanId: 200, innerVlanId: 100 }))).toBe(false);
    expect(sameTagStack(a, tagStack({ vlanId: 200, innerVlanId: 101, dot1ad: true }))).toBe(false);
    expect(sameTagStack(a, tagStack({ vlanId: 200, dot1ad: true }))).toBe(false);
  });

  it('live stack: numbers from the live state, the tag type from the retrieved config, else from running', () => {
    const state = { vlanId: 200, innerVlanId: 100 };
    expect(
      formatTagStack(
        liveTagStack({ state, config: { dot1ad: true }, running: { dot1ad: false } })!,
      ),
    ).toBe('dot1ad 200 · dot1q 100');
    expect(formatTagStack(liveTagStack({ state, config: null, running: { dot1ad: true } })!)).toBe(
      'dot1ad 200 · dot1q 100',
    );
    expect(formatTagStack(liveTagStack({ state, config: null, running: null })!)).toBe(
      'dot1q 200 · dot1q 100',
    );
    expect(
      formatTagStack(liveTagStack({ state: { vlanId: 100, innerVlanId: 0 }, config: {} })!),
    ).toBe('dot1q 100');
    expect(liveTagStack({ state: null, config: { vlanId: 100 } })).toBeUndefined();
    expect(liveTagStack(undefined)).toBeUndefined();
  });
});
