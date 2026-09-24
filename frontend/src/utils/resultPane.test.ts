// issue #9（Tester 角色，RED）：结果区纯逻辑模块（设计 §1/§3/§4 测试 (b)）。
//
// 实现必须把结果区的纯逻辑抽到 frontend/src/utils/resultPane.ts（本文件按其
// 导出名 import），TranslateWindow 消费它们：
//   - activeEngineFor(lastUsed, defaultEngine, engines)：
//       resolvePrimaryEngine(lastUsed, defaultEngine, engines)
//       || 第一个 enabled translate 引擎（防御性回退，保证 select 永不悬空）；
//     解析规则不在此处重新发明——必须消费 #8 的 resolvePrimaryEngine。
//   - statusDot(engine, results, loading) / statusDots(engines, results, loading)：
//     每个 enabled translate 引擎一个 dot，'pending' | 'done' | 'failed'：
//       done    = results[engine]?.result 为非空串；
//       pending = loading === true 且无（非空）结果；
//       failed  = !loading 且无（非空）结果（失败的引擎后端不发出任何事件，
//                  它在 results 里就是缺席的——设计 §4 的 failed 是派生信号）。
//   - resetEdits(edited, previousEngine, nextEngine, results)：切换引擎时丢弃
//     上一引擎的手工编辑，新引擎从其存储的 result 开始（设计 §3「manual edits
//     reset」：无 per-engine 编辑记忆）。
//   - anyPending(engines, results)：15 s 回退谓词「是否仍有引擎没有结果」
//     （设计 §4 扩展：从「零结果」放宽到「任一 pending」）。
//
// 无 mock：真实 jsdom localStorage（NODE_OPTIONS=--localstorage-file，见
// vitest.config.ts 头注）、真实 setTimeout（Node 真计时器；15 s 用例用真实
// 等待，不 mock 定时器、不 stub wails runtime）。
//
// RED 说明：./resultPane.ts 模块尚不存在（结果区逻辑今天内联在
// TranslateWindow.svelte 里）——本文件 import 即失败，vitest 报出
// 「Failed to resolve import ... resultPane.ts」。这是「行为缺失」的 RED。
// 本文件不触碰任何现有测试/模块；实现落地后本文件应全部转绿。

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import {
  activeEngineFor,
  statusDot,
  statusDots,
  resetEdits,
  anyPending,
  type PaneEngine,
  type PaneResult,
} from './resultPane.ts';

const LAST_USED_KEY = 'kai:translate:lastEngine';

// 与 #8 前端镜像测试同一组引擎（configstore id 顺序：google=1, deepl=2）。
const ENGINES: PaneEngine[] = [
  { id: 1, value: 'google', name: 'google', kind: 'translate', enabled: true, supported: true },
  { id: 2, value: 'deepl', name: 'deepl', kind: 'translate', enabled: true, supported: true },
];

const OCR_ENGINE: PaneEngine = {
  id: 3,
  value: 'tesseract',
  name: 'tesseract',
  kind: 'ocr',
  enabled: true,
  supported: true,
};

const GOOGLE_DISABLED: PaneEngine[] = ENGINES.map((e) =>
  e.value === 'google' ? { ...e, enabled: false } : e,
);

beforeEach(() => {
  // 真实 localStorage（Node 26 需 --localstorage-file，见 vitest.config.ts）。
  window.localStorage.removeItem(LAST_USED_KEY);
});

// 收尾清理：--localstorage-file 是文件后端 store，removeItem 的删除可能以空串
// 形式残留（resolvePrimaryEngine 的「no-op on empty」守卫会跳过它），并在
// 后续测试文件（resolvePrimaryEngine.test.ts 等）的 vitest worker 中跨文件泄漏。
// 用 clear() 兜底：每个测试结束后不向 store 留下任何状态，文件间互不污染。
afterEach(() => {
  window.localStorage.clear();
});

describe('activeEngineFor（消费 #8 resolvePrimaryEngine + 防御性回退）', () => {
  it('last-used 在 enabled translate 引擎中时胜出（覆盖 primary）', () => {
    window.localStorage.setItem(LAST_USED_KEY, JSON.stringify('deepl'));
    expect(activeEngineFor(LAST_USED_KEY, 'google', ENGINES)).toBe('deepl');
  });

  it('last-used 引擎被禁用时回退到 primary', () => {
    window.localStorage.setItem(LAST_USED_KEY, JSON.stringify('google'));
    expect(activeEngineFor(LAST_USED_KEY, 'deepl', GOOGLE_DISABLED)).toBe('deepl');
  });

  it('last-used 与 primary 都非法时回退到第一个 enabled 的 translate 引擎', () => {
    expect(activeEngineFor(LAST_USED_KEY, 'nosuchengine', ENGINES)).toBe('google');
  });

  it('都没有时回退到第一个 enabled 的 translate 引擎（id 顺序）', () => {
    expect(activeEngineFor(LAST_USED_KEY, '', ENGINES)).toBe('google');
  });

  it('primary 指向 ocr 引擎时不算有效 primary', () => {
    expect(activeEngineFor(LAST_USED_KEY, 'tesseract', [...ENGINES, OCR_ENGINE])).toBe('google');
  });

  it('防御性回退：resolvePrimaryEngine 返回 "" 但有 enabled 引擎时，取第一个 enabled（select 不悬空）', () => {
    // 让 #8 镜像返回 ''：localStorage 里 last-used 是损坏/非法值且 primary 为空、
    // 同时构造一个 resolvePrimaryEngine 会落到 '' 的引擎列表不可能存在
    // （有 enabled 引擎时它必返回第一个）——因此该防御分支只对「resolve 返回 ''
    // 且列表非空」这种未来规则变更防御；此处用全部禁用的列表验证 activeEngineFor
    // 返回 ''（没有可回退对象时也只能是空，行为定义：返回 resolve 原值）。
    const noneEnabled = ENGINES.map((e) => ({ ...e, enabled: false }));
    expect(activeEngineFor(LAST_USED_KEY, '', noneEnabled)).toBe('');
  });
});

describe('statusDot / statusDots（pending / done / failed 派生自真实 fan-out 信号）', () => {
  it('loading 且尚无该引擎结果 -> pending', () => {
    expect(statusDot('google', {}, true)).toBe('pending');
    expect(statusDot('deepl', {}, true)).toBe('pending');
  });

  it('非空 result -> done（loading 与否都成立：结果到了就是 done）', () => {
    const results: Record<string, PaneResult> = { google: { result: '你好' } };
    expect(statusDot('google', results, true)).toBe('done');
    expect(statusDot('google', results, false)).toBe('done');
  });

  it('空串 result 不算 done：!loading 且无可用结果 -> failed', () => {
    expect(statusDot('google', { google: { result: '' } }, false)).toBe('failed');
  });

  it('!loading 且该引擎缺席（失败引擎后端不发事件）-> failed', () => {
    const results: Record<string, PaneResult> = { deepl: { result: 'hallo' } };
    expect(statusDot('google', results, false)).toBe('failed');
  });

  it('loading 时缺席引擎仍是 pending（15 s 回退前）', () => {
    const results: Record<string, PaneResult> = { deepl: { result: 'hallo' } };
    expect(statusDot('google', results, true)).toBe('pending');
  });

  it('case 1：多引擎 fan-out 中一个失败、兄弟引擎成功（翻 loading=false）-> 失败 dot 立即 failed', () => {
    const results: Record<string, PaneResult> = { deepl: { result: 'hallo' } };
    const dots = statusDots(ENGINES, results, false);
    expect(dots.google).toBe('failed');
    expect(dots.deepl).toBe('done');
  });

  it('case 2：唯一引擎失败时，15 s 回退前 loading 仍为 true -> dot 为 pending', () => {
    const sole = [ENGINES[0]];
    const dots = statusDots(sole, {}, true);
    expect(dots.google).toBe('pending');
  });

  it('statusDots 只覆盖 enabled 的 translate 引擎（ocr / 禁用引擎不出 dot）', () => {
    const all = [...ENGINES, OCR_ENGINE];
    const dots = statusDots(all, {}, true);
    expect(Object.keys(dots).sort()).toEqual(['deepl', 'google']);
  });

  it('anyPending：存在无结果引擎 -> true（15 s 回退谓词：任一 pending 即维持 loading）', () => {
    expect(anyPending(ENGINES, {}, true)).toBe(true);
    expect(anyPending(ENGINES, { google: { result: 'x' } }, true)).toBe(true);
  });

  it('anyPending：所有引擎都有非空结果 -> false（回退不再翻 loading）', () => {
    expect(anyPending(ENGINES, { google: { result: 'x' }, deepl: { result: 'y' } }, false)).toBe(
      false,
    );
  });
});

describe('resetEdits（切换引擎丢弃上一引擎的手工编辑）', () => {
  it('丢弃 previous 的编辑；next 从其存储 result 开始（无 per-engine 编辑记忆）', () => {
    const results: Record<string, PaneResult> = {
      google: { result: '你好' },
      deepl: { result: 'hallo' },
    };
    const edited = new Map<string, string>([['google', '手工改过的文本']]);
    const next = resetEdits(edited, 'google', 'deepl', results);
    // google 的编辑被丢弃。
    expect(next.get('google')).toBeUndefined();
    // deepl 的显示文本回到其存储 result（编辑 map 里没有它，显示层取 result）。
    expect(next.get('deepl')).toBeUndefined();
    expect(next.get('deepl') ?? results['deepl']?.result).toBe('hallo');
  });

  it('不修改传入的旧 map（返回新 Map）', () => {
    const results: Record<string, PaneResult> = { google: { result: 'a' }, deepl: { result: 'b' } };
    const edited = new Map<string, string>([['google', 'edit']]);
    const before = new Map(edited);
    resetEdits(edited, 'google', 'deepl', results);
    expect(edited).toEqual(before);
  });

  it('previous === next（同引擎重选）也按切换处理：该引擎的编辑被清空、回到存储 result', () => {
    const results: Record<string, PaneResult> = { google: { result: '你好' } };
    const edited = new Map<string, string>([['google', '改过']]);
    const next = resetEdits(edited, 'google', 'google', results);
    expect(next.get('google')).toBeUndefined();
  });
});
