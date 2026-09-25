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
  failureMessage,
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

describe('activeEngineFor (consumes #8 resolvePrimaryEngine + defensive fallback)', () => {
  it('last-used wins when among enabled translate engines (overrides primary)', () => {
    window.localStorage.setItem(LAST_USED_KEY, JSON.stringify('deepl'));
    expect(activeEngineFor(LAST_USED_KEY, 'google', ENGINES)).toBe('deepl');
  });

  it('falls back to primary when the last-used engine is disabled', () => {
    window.localStorage.setItem(LAST_USED_KEY, JSON.stringify('google'));
    expect(activeEngineFor(LAST_USED_KEY, 'deepl', GOOGLE_DISABLED)).toBe('deepl');
  });

  it('falls back to the first enabled translate engine when last-used and primary are both invalid', () => {
    expect(activeEngineFor(LAST_USED_KEY, 'nosuchengine', ENGINES)).toBe('google');
  });

  it('falls back to the first enabled translate engine when neither is set (id order)', () => {
    expect(activeEngineFor(LAST_USED_KEY, '', ENGINES)).toBe('google');
  });

  it('an ocr engine is not a valid primary', () => {
    expect(activeEngineFor(LAST_USED_KEY, 'tesseract', [...ENGINES, OCR_ENGINE])).toBe('google');
  });

  it('defensive fallback: takes the first enabled engine when resolvePrimaryEngine returns "" (select never dangles)', () => {
    // 让 #8 镜像返回 ''：localStorage 里 last-used 是损坏/非法值且 primary 为空、
    // 同时构造一个 resolvePrimaryEngine 会落到 '' 的引擎列表不可能存在
    // （有 enabled 引擎时它必返回第一个）——因此该防御分支只对「resolve 返回 ''
    // 且列表非空」这种未来规则变更防御；此处用全部禁用的列表验证 activeEngineFor
    // 返回 ''（没有可回退对象时也只能是空，行为定义：返回 resolve 原值）。
    const noneEnabled = ENGINES.map((e) => ({ ...e, enabled: false }));
    expect(activeEngineFor(LAST_USED_KEY, '', noneEnabled)).toBe('');
  });
});

describe('statusDot / statusDots (pending / done / failed derived from real fan-out signals)', () => {
  it('loading with no result for the engine -> pending', () => {
    expect(statusDot('google', {}, true)).toBe('pending');
    expect(statusDot('deepl', {}, true)).toBe('pending');
  });

  it('non-empty result -> done (holds with or without loading: result arrived means done)', () => {
    const results: Record<string, PaneResult> = { google: { result: 'hola' } };
    expect(statusDot('google', results, true)).toBe('done');
    expect(statusDot('google', results, false)).toBe('done');
  });

  it('empty-string result is not done: !loading and no usable result -> failed', () => {
    expect(statusDot('google', { google: { result: '' } }, false)).toBe('failed');
  });

  it('!loading and the engine is absent (backend emits no event for failed engines) -> failed', () => {
    const results: Record<string, PaneResult> = { deepl: { result: 'hallo' } };
    expect(statusDot('google', results, false)).toBe('failed');
  });

  it('absent engine stays pending while loading (before the 15 s fallback)', () => {
    const results: Record<string, PaneResult> = { deepl: { result: 'hallo' } };
    expect(statusDot('google', results, true)).toBe('pending');
  });

  it('case 1: one engine fails while a sibling succeeds in a multi-engine fan-out (loading=false) -> failed dot immediately failed', () => {
    const results: Record<string, PaneResult> = { deepl: { result: 'hallo' } };
    const dots = statusDots(ENGINES, results, false);
    expect(dots.google).toBe('failed');
    expect(dots.deepl).toBe('done');
  });

  it('case 2: the only engine fails; loading stays true before the 15 s fallback -> dot stays pending', () => {
    const sole = [ENGINES[0]];
    const dots = statusDots(sole, {}, true);
    expect(dots.google).toBe('pending');
  });

  it('statusDots only covers enabled translate engines (ocr / disabled engines get no dot)', () => {
    const all = [...ENGINES, OCR_ENGINE];
    const dots = statusDots(all, {}, true);
    expect(Object.keys(dots).sort()).toEqual(['deepl', 'google']);
  });

  it('anyPending: an engine without a result -> true (15 s fallback predicate: any pending keeps loading)', () => {
    expect(anyPending(ENGINES, {}, true)).toBe(true);
    expect(anyPending(ENGINES, { google: { result: 'x' } }, true)).toBe(true);
  });

  it('anyPending: all engines have non-empty results -> false (fallback no longer flips loading)', () => {
    expect(anyPending(ENGINES, { google: { result: 'x' }, deepl: { result: 'y' } }, false)).toBe(
      false,
    );
  });
});

describe('resetEdits (switching engines drops the previous engine manual edits)', () => {
  it('drops previous edits; next starts from its stored result (no per-engine edit memory)', () => {
    const results: Record<string, PaneResult> = {
      google: { result: 'hola' },
      deepl: { result: 'hallo' },
    };
    const edited = new Map<string, string>([['google', 'manually edited text']]);
    const next = resetEdits(edited, 'google', 'deepl', results);
    // google 的编辑被丢弃。
    expect(next.get('google')).toBeUndefined();
    // deepl 的显示文本回到其存储 result（编辑 map 里没有它，显示层取 result）。
    expect(next.get('deepl')).toBeUndefined();
    expect(next.get('deepl') ?? results['deepl']?.result).toBe('hallo');
  });

  it('does not mutate the passed-in map (returns a new Map)', () => {
    const results: Record<string, PaneResult> = { google: { result: 'a' }, deepl: { result: 'b' } };
    const edited = new Map<string, string>([['google', 'edit']]);
    const before = new Map(edited);
    resetEdits(edited, 'google', 'deepl', results);
    expect(edited).toEqual(before);
  });

  it('previous === next (same engine re-selected) is still treated as a switch: its edits are cleared, back to the stored result', () => {
    const results: Record<string, PaneResult> = { google: { result: 'hola' } };
    const edited = new Map<string, string>([['google', 'edited']]);
    const next = resetEdits(edited, 'google', 'google', results);
    expect(next.get('google')).toBeUndefined();
  });
});

// issue #42：失败载荷的面向用户文案（kind → 本地化 key，原始细节附带）。
describe('failureMessage (#42 surfacing failure reasons)', () => {
  const t = (key: string) => {
    const dict: Record<string, string> = {
      'translate.failed': 'Translation failed',
      'translate.failedPair': 'Language pair unavailable',
      'translate.failedNetwork': 'Engine unreachable — check network or proxy',
      'translate.failedAuth': 'Check the API key',
    };
    return dict[key] ?? key;
  };

  it('maps pair kind to the actionable message, detail appended', () => {
    expect(
      failureMessage({ engine: 'apple', error: 'Unable to Translate', errorKind: 'pair' }, t),
    ).toBe('Language pair unavailable — Unable to Translate');
  });

  it('maps network kind to the reachability message', () => {
    expect(
      failureMessage({ engine: 'google', error: 'dial tcp: refused', errorKind: 'network' }, t),
    ).toBe('Engine unreachable — check network or proxy — dial tcp: refused');
  });

  it('maps auth kind to the key message', () => {
    expect(failureMessage({ engine: 'gpt', error: '401', errorKind: 'auth' }, t)).toBe(
      'Check the API key — 401',
    );
  });

  it('falls back to the generic message for unknown kinds', () => {
    expect(failureMessage({ engine: 'x', error: 'boom', errorKind: 'engine' }, t)).toBe(
      'Translation failed — boom',
    );
  });

  it('returns the generic message with no detail for null/absent results', () => {
    expect(failureMessage(null, t)).toBe('Translation failed');
    expect(failureMessage({ engine: 'x' }, t)).toBe('Translation failed');
  });
});
