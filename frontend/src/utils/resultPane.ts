// issue #9：结果区纯逻辑（从 TranslateWindow.svelte 抽出的无 DOM 部分，供测试独立验证）。
//
// 解析规则不在这里重新发明：activeEngineFor 直接消费 #8 的 resolvePrimaryEngine
// （last-used ?? primary(default_engine) ?? first-enabled），只在其之上加一层
// 「'' → 第一个 enabled translate 引擎」的防御性回退，保证结果区的引擎 select
// 永不悬空。
//
// 状态 dot 的三态是 fan-out 真实输出的纯函数（设计 §4，后端不变）：
//   done    = results[engine].result 为非空串（结果到达即 done，与 loading 无关）；
//   pending = loading 且该引擎尚无（非空）结果；
//   failed  = !loading 且该引擎无（非空）结果——失败的引擎后端不发任何事件，
//              它在 results 里就是缺席的，failed 是「缺席 + loading 已结束」的派生信号。
//
// 本模块不触碰 localStorage / wails runtime / DOM：TranslateWindow 持有 lastUsed 的
// 持久化 store，把解析输入交给 activeEngineFor；结果区模板消费 statusDots 与 resetEdits。

import { resolvePrimaryEngine } from './resolvePrimaryEngine.ts';

/** 结果区引擎条目（GetAllEngines 形态）：id 顺序即「第一个」的顺序。 */
export type PaneEngine = {
  id: number;
  value: string;
  name: string;
  kind: string;
  enabled: boolean;
  supported: boolean;
  builtin?: boolean;
};

/** 单引擎 fan-out 结果条目（TranslateResult 的形态，result 为译文；空串/缺席即无结果）。 */
export type PaneResult = {
  engine?: string;
  result?: string;
  phonetic?: string;
  /** issue #42：失败载荷的原始错误与类别（pair/network/auth/engine）；成功时缺席。 */
  error?: string;
  errorKind?: string;
  [key: string]: unknown;
};

/**
 * 失败载荷的面向用户文案（issue #42）：kind → 本地化 key（pair/network/auth 有
 * 可操作文案，其余回退 translate.failed），原始错误细节以「 — 」附于其后。
 * 纯函数：t（i18n 取词）由调用方注入。
 */
export function failureMessage(
  result: PaneResult | null | undefined,
  t: (key: string) => string,
): string {
  const generic = t('translate.failed');
  if (!result?.error) return generic;
  const key =
    result.errorKind === 'pair'
      ? 'translate.failedPair'
      : result.errorKind === 'network'
        ? 'translate.failedNetwork'
        : result.errorKind === 'auth'
          ? 'translate.failedAuth'
          : 'translate.failed';
  return `${t(key)} — ${result.error}`;
}

/**
 * 「enabled 的 translate 引擎」谓词：kind=translate 且 enabled 且平台支持。
 * Exported so the target-language capability gating (issue #52, utils/targetCapability.ts)
 * shares this one definition instead of restating it.
 */
export function isEnabledTranslate(e: Pick<PaneEngine, 'kind' | 'enabled' | 'supported'>): boolean {
  return e.kind === 'translate' && e.enabled && e.supported;
}

/**
 * 解析结果区当前绑定的引擎 name（= #8 的 resolvedPrimary 推导 + 防御性回退）。
 *
 * @param lastUsedKey   localStorage key（如 `kai:translate:lastEngine`），resolvePrimaryEngine
 *                      自行读取（真实 localStorage；损坏/空值回退下一层）。
 * @param defaultEngine settings 的 default_engine（primary，解析链中间层）。
 * @param engines       GetAllEngines 形态的引擎列表（id 顺序）。
 * @returns 解析出的引擎 name；resolvePrimaryEngine 返回 '' 而列表里仍有 enabled
 *          translate 引擎时（对未来规则变更的防御），回退到第一个 enabled 的
 *          translate 引擎，保证 select 不悬空；列表里没有任何可用引擎时返回 ''
 *          （面板展示既有「no active engine」空态）。
 */
export function activeEngineFor(
  lastUsedKey: string,
  defaultEngine: string,
  engines: PaneEngine[],
): string {
  const resolved = resolvePrimaryEngine(lastUsedKey, defaultEngine, engines);
  if (resolved) return resolved;
  for (const e of engines) {
    if (isEnabledTranslate(e)) return e.value;
  }
  return '';
}

/** 单个引擎的 dot 状态（设计 §4 的状态表）。 */
export type DotState = 'pending' | 'done' | 'failed';

/**
 * 单个引擎的 dot 状态：done（非空结果）> pending（loading 且无结果）> failed
 * （!loading 且无结果——失败引擎缺席于 results）。
 */
export function statusDot(
  engine: string,
  results: Record<string, PaneResult>,
  loading: boolean,
): DotState {
  if (results[engine]?.result) return 'done';
  if (loading) return 'pending';
  return 'failed';
}

/**
 * 每个 enabled translate 引擎一个 dot（activeEngines 顺序，即 id 顺序）；
 * ocr / 禁用引擎不出 dot（设计 §4）。
 */
export function statusDots(
  engines: PaneEngine[],
  results: Record<string, PaneResult>,
  loading: boolean,
): Record<string, DotState> {
  const dots: Record<string, DotState> = {};
  for (const e of engines) {
    if (!isEnabledTranslate(e)) continue;
    dots[e.value] = statusDot(e.value, results, loading);
  }
  return dots;
}

/**
 * 15 s 回退谓词（设计 §4 扩展）：fan-out 是否仍在进行中——即存在某个 enabled
 * translate 引擎「仍在 pending」（loading 且尚无（非空）结果）。
 * 旧逻辑只在「零结果」时解除 loading；放宽到「任一 pending」后，兄弟引擎到达会
 * 各自把 loading 翻 false，失败引擎缺席、loading 解除的同一刻其 dot 由 pending
 * 翻转为 failed（case 1 即时，case 2 由本谓词在 15 s 兜底点收敛）。
 * `loading === false` 时没有任何 pending 引擎 -> 恒 false（此时回退无意义）。
 */
export function anyPending(
  engines: PaneEngine[],
  results: Record<string, PaneResult>,
  loading: boolean,
): boolean {
  if (!loading) return false;
  for (const e of engines) {
    if (isEnabledTranslate(e) && !results[e.value]?.result) return true;
  }
  return false;
}

/**
 * 引擎切换（或重选同一引擎）时的编辑重置（设计 §3「manual edits reset」）：
 * 返回一个新 Map，丢弃 previousEngine 的手工编辑（无 per-engine 编辑记忆）；
 * nextEngine 的显示文本回到其存储的 result（map 里没有它，显示层 ?? 兜底）。
 * 入参的旧 Map 不被修改。
 */
export function resetEdits(
  edited: Map<string, string>,
  previousEngine: string,
  nextEngine: string,
  _results: Record<string, PaneResult>,
): Map<string, string> {
  const next = new Map(edited);
  next.delete(previousEngine);
  return next;
}
