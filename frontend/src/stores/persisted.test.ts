// issue #7（Tester 角色，RED）：前端 vitest 示例测试。
//
// 为什么选 persisted store（而不是 lang.ts 的 lang-name 回退）：
//  - lang.ts 把 @bindings/.../model/models.ts 的 wails 生成 enum `Language`
//    re-export 成 TRANSLATE_LANG / ALL_TRANSLATE_LANGS / TARGET_TRANSLATE_LANGS，
//    整个模块身份依赖 wails 生成物，测试环境无 wails runtime，import 即解析失败。
//  - persisted.ts 只 import svelte/store，是纯逻辑 + 真实 localStorage，
//    正好对应 issue 约定「frontend logic in jsdom with real localStorage」。
//
// 约束：
//  - jsdom 提供真实 window.localStorage，不 mock；
//  - 无组件测试框架；
//  - 每个用例用唯一 key 隔离，互不干扰。
import { describe, it, expect, beforeEach } from 'vitest';
import { persisted, pinKey } from './persisted.ts';

const KEY = 'kai:test:persisted';

beforeEach(() => {
  // 清理上一用例写入的 localStorage，保证每个用例从空状态开始。
  window.localStorage.removeItem(KEY);
  window.localStorage.removeItem(pinKey('translate'));
});

describe('persisted store（jsdom + 真实 localStorage）', () => {
  it('localStorage 无值时回退 initial', () => {
    const s = persisted<number>(KEY, 42);
    let got = 0;
    const unsub = s.subscribe((v) => (got = v));
    expect(got).toBe(42);
    unsub();
  });

  it('初始化时从 localStorage 读回上次持久化的值', () => {
    // 模拟「上次运行」写入
    window.localStorage.setItem(KEY, JSON.stringify(7));
    const s = persisted<number>(KEY, 42);
    let got = 0;
    const unsub = s.subscribe((v) => (got = v));
    expect(got).toBe(7);
    unsub();
  });

  it('set 后 localStorage 同步更新，新 store 能读回', () => {
    const s1 = persisted<number>(KEY, 1);
    s1.set(99);
    // 新 store 用同 key 重新初始化 → 应读到 99 而非 initial 1
    const s2 = persisted<number>(KEY, 1);
    let got = 0;
    const unsub = s2.subscribe((v) => (got = v));
    expect(got).toBe(99);
    expect(JSON.parse(window.localStorage.getItem(KEY)!)).toBe(99);
    unsub();
  });

  it('localStorage 值损坏（非法 JSON）时回退 initial', () => {
    window.localStorage.setItem(KEY, '{not-json');
    const s = persisted<number>(KEY, 5);
    let got = 0;
    const unsub = s.subscribe((v) => (got = v));
    expect(got).toBe(5);
    unsub();
  });

  it('pinKey 按窗口名生成独立 key', () => {
    expect(pinKey('translate')).toBe('kai:translate:pinned');
    expect(pinKey('settings')).toBe('kai:settings:pinned');
    expect(pinKey('translate')).not.toBe(pinKey('settings'));
  });
});
