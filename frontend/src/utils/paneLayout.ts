// 两栏布局的分隔条数学（issue #10）：把「拖拽点 → 左栏宽度占比」收敛成一个纯函数，
// 便于 vitest 单测（真实拖拽在 WKWebView 里无法自动化，见 issue #28）。
// 组件侧只做事件绑定，所有夹逼逻辑都在这里。

/** 左栏最小占比（窗口再窄，源文本栏也不小于整个内容区的 1/4）。 */
export const MIN_RATIO = 0.25;
/** 左栏最大占比（结果栏同理保底 1/4）。 */
export const MAX_RATIO = 0.75;

/**
 * 把任意占比夹逼到 [min, max]。NaN（拖拽事件里的坐标异常）回退到中点 0.5，
 * 保证分隔条永远停留在合法位置而不是消失。
 */
export function clampRatio(
  ratio: number,
  min: number = MIN_RATIO,
  max: number = MAX_RATIO,
): number {
  if (Number.isNaN(ratio)) return 0.5;
  return Math.min(max, Math.max(min, ratio));
}

/**
 * 把「指针在布局行内的水平位置」换算成左栏占比。
 * x 超出行边界时夹逼到两端；width 为 0（布局尚未完成测量）时回退中点。
 */
export function ratioFromPoint(
  x: number,
  left: number,
  width: number,
  min: number = MIN_RATIO,
  max: number = MAX_RATIO,
): number {
  if (width <= 0) return 0.5;
  return clampRatio((x - left) / width, min, max);
}
