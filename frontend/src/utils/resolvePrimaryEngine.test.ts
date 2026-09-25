// issue #8（Tester 角色，RED）：翻译窗口首屏主引擎解析规则（前端镜像）。
//
// 规则（与设计文档 §4 的 Go 权威实现 PrimaryTranslateEngine 对齐，test d 的
// divergence check 保证两侧一致）：
//
//   解析顺序 last-used ?? primary ?? first-enabled：
//   1. lastUsed（localStorage 持久化的上次使用引擎）在 enabled translate 引擎中 -> 它；
//   2. 否则 settings 的 default_engine 有效（在列表中、kind=translate、enabled）-> 它；
//   3. 否则第一个 enabled 的 translate 引擎（列表 id 顺序）；
//   4. 没有 enabled 的 translate 引擎 -> ''。
//
// 「enabled translate」谓词 = kind === 'translate' && enabled && supported
// （与 Go 侧 GetAllEngines 形态一致；见设计 §3）。
//
// RED 说明：resolvePrimaryEngine 尚不存在（设计 §4 的前端镜像模块），本文件
// import 即解析失败。这是「行为缺失」的 RED，不是环境或拼写问题。
// 引擎列表与 Go 侧 (b)/(c) 用同一组数据，Go 侧测试解析结果必须与本文件一致。
import { describe, it, expect, beforeEach } from 'vitest';
import { resolvePrimaryEngine } from './resolvePrimaryEngine.ts';

const LAST_USED_KEY = 'kai:translate:lastEngine';

// 与 Go 侧 (b)/(c) 相同的引擎列表（configstore id 顺序：google=1, deepl=2）。
const ENGINES = [
  {
    id: 1,
    value: 'google',
    name: 'google',
    kind: 'translate',
    enabled: true,
    supported: true,
    builtin: false,
  },
  {
    id: 2,
    value: 'deepl',
    name: 'deepl',
    kind: 'translate',
    enabled: true,
    supported: true,
    builtin: false,
  },
];

// 把 google 禁用后的列表（对应 Go 侧 test c 的中段状态）。
const ENGINES_GOOGLE_DISABLED = ENGINES.map((e) =>
  e.value === 'google' ? { ...e, enabled: false } : e,
);

const ocrItem = {
  id: 3,
  value: 'tesseract',
  name: 'tesseract',
  kind: 'ocr',
  enabled: true,
  supported: true,
  builtin: false,
};

beforeEach(() => {
  window.localStorage.removeItem(LAST_USED_KEY);
});

describe('resolvePrimaryEngine (last-used ?? primary ?? first-enabled, real localStorage)', () => {
  it('last-used wins when it is among enabled translate engines', () => {
    // 真实写入 localStorage（persisted store 的写入路径同形）。
    window.localStorage.setItem(LAST_USED_KEY, JSON.stringify('deepl'));
    expect(resolvePrimaryEngine(LAST_USED_KEY, 'google', ENGINES)).toBe('deepl');
  });

  it('falls back to primary when the last-used engine is disabled', () => {
    window.localStorage.setItem(LAST_USED_KEY, JSON.stringify('google'));
    // google 已禁用（与 Go 侧 test c 同一列表状态），primary 仍是 deepl。
    expect(resolvePrimaryEngine(LAST_USED_KEY, 'deepl', ENGINES_GOOGLE_DISABLED)).toBe('deepl');
  });

  it('falls back to the first enabled translate engine when primary is invalid (not in the list)', () => {
    expect(resolvePrimaryEngine(LAST_USED_KEY, 'nosuchengine', ENGINES)).toBe('google');
  });

  it('falls back to the first enabled translate engine when last-used and primary are both unset', () => {
    expect(resolvePrimaryEngine(LAST_USED_KEY, '', ENGINES)).toBe('google');
  });

  it('an ocr engine is not a valid primary; falls back to first-enabled', () => {
    const withOcr = [...ENGINES, ocrItem];
    expect(resolvePrimaryEngine(LAST_USED_KEY, 'tesseract', withOcr)).toBe('google');
  });

  it('returns an empty string when no translate engine is enabled', () => {
    const noneEnabled = ENGINES.map((e) => ({ ...e, enabled: false }));
    expect(resolvePrimaryEngine(LAST_USED_KEY, '', noneEnabled)).toBe('');
  });

  it('reads real localStorage: an empty-string last-used does not override primary', () => {
    // 真实读回（模拟 persisted store 从 localStorage 初始化的路径）。
    window.localStorage.setItem(LAST_USED_KEY, JSON.stringify(''));
    expect(resolvePrimaryEngine(LAST_USED_KEY, 'deepl', ENGINES)).toBe('deepl');
  });
});
