// issue #8：翻译窗口首屏「主翻译引擎」解析（前端镜像，纯函数）。
//
// 权威实现在 Go 侧 EngineWrapper.PrimaryTranslateEngine；本函数是其前端镜像，
// 用前端自有的引擎列表（GetAllEngines 形态）做同样的解析，使首屏不必等待一次
// 额外的后端往返。test (d) 的 divergence check 保证两侧对同一引擎列表解析一致。
//
// 解析顺序 last-used ?? primary ?? first-enabled：
//   1. lastUsed（localStorage 持久化的上次使用引擎）在 enabled translate 引擎中 -> 它；
//   2. 否则 defaultEngine 有效（在列表中、kind=translate、enabled）-> 它；
//   3. 否则第一个 enabled 的 translate 引擎（列表 id 顺序）；
//   4. 没有 enabled 的 translate 引擎 -> ''。
//
// 「enabled translate」谓词 = kind === 'translate' && enabled && supported
// （与 Go 侧 GetAllEngines 形态一致，见设计 §3）。

/** GetAllEngines 条目（bindings 形态）：含 enabled / kind / supported，比 EngineListItem 更全。 */
type PrimaryEngineItem = {
  id: number;
  value: string;
  name: string;
  kind: string;
  enabled: boolean;
  supported: boolean;
  builtin?: boolean;
};

/** 「enabled 的 translate 引擎」谓词（前端镜像的判定核心）。 */
function isEnabledTranslate(e: PrimaryEngineItem): boolean {
  return e.kind === 'translate' && e.enabled && e.supported;
}

/**
 * 解析翻译窗口首屏应绑定的主翻译引擎 name。
 *
 * @param lastUsedKey  localStorage key（如 `kai:translate:lastEngine`），持久化上次使用的引擎；
 *                      为空串 / 未设置 / 引擎已失效时跳过该层。
 * @param defaultEngine settings 的 default_engine（主引擎）；未设置 / 非法时跳过该层。
 * @param engines        GetAllEngines 形态的引擎列表（id 顺序即「第一个」的顺序）。
 * @returns 解析出的引擎 name；无任何可用的 enabled translate 引擎时返回 ''。
 */
export function resolvePrimaryEngine(
  lastUsedKey: string,
  defaultEngine: string,
  engines: PrimaryEngineItem[],
): string {
  // 1. last-used 胜出（读真实 localStorage；空串 / 损坏值 / 失效引擎都回退下一层）。
  let lastUsed = '';
  try {
    const raw = window.localStorage.getItem(lastUsedKey);
    if (raw !== null) {
      const parsed = JSON.parse(raw);
      if (typeof parsed === 'string') lastUsed = parsed;
    }
  } catch {
    // 忽略损坏值，回退到下一层。
  }
  if (lastUsed) {
    const hit = engines.find((e) => e.value === lastUsed && isEnabledTranslate(e));
    if (hit) return hit.value;
  }

  // 2. settings 的 default_engine 有效则胜出。
  if (defaultEngine) {
    const hit = engines.find((e) => e.value === defaultEngine && isEnabledTranslate(e));
    if (hit) return hit.value;
  }

  // 3. 回退到列表里第一个 enabled 的 translate 引擎（id 顺序）。
  for (const e of engines) {
    if (isEnabledTranslate(e)) return e.value;
  }

  // 4. 没有可用的 enabled translate 引擎。
  return '';
}
