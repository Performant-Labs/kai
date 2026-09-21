import { emitEvent } from '../runtime';

// track 上报一个匿名统计事件（event + 属性）。
// dev 构建 / 未配置 key / 用户关闭开关时，Go 侧 analytics.Track 会自动 no-op，前端无需判断。
export function track(event: string, props: Record<string, unknown> = {}): void {
  emitEvent('kai:analytics:track', { event, props });
}
