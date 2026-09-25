// vitest 配置（issue #7 前端测试底座）。
// 独立于 vite.config.ts：后者加载 @wailsio/runtime 的 wails vite 插件并生成
// bindings，依赖 wails runtime；而单元测试跑在无 wails 的 vitest 进程里，
// 只需 jsdom（真实 window.localStorage / navigator），且被测的 persisted store
// 是纯 svelte/store + localStorage 逻辑，不涉及 svelte runes 编译。
//
// Node 26 localStorage 说明（issue #7）：舰队镜像 pl-runner:1.70.1 烤的是 Node
// v26，它把 localStorage / sessionStorage 注册成 globalThis 的 own 属性（实验性
// webstorage 的惰性 getter，缺 --localstorage-file 时永远返回 undefined）。vitest
// 的 jsdom 环境在 populateGlobal 里对「已存在于 globalThis 的 key」一律跳过，
// 因此 Node 的惰性 getter 留在 globalThis 上，测试里 window.localStorage 是
// undefined。解法不在这里：ci-go.sh 为前端测试导出
// NODE_OPTIONS=--localstorage-file=$HOME/.kai-localstorage.json，让 Node 的原生
// getter 返回真实 Storage（vitest 原样保留它，window===globalThis 故测试直接命中）。
// 本地开发跑 `make test-frontend` 前也需同样 export，否则 Node 26 上必挂。
import { resolve } from 'node:path';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  // issue #52: lang.ts / i18n import the generated model enum via @bindings (a plain,
  // runtime-light TS file), so unit tests need the same alias vite.config.ts declares.
  resolve: { alias: { '@bindings': resolve(__dirname, 'bindings') } },
  test: {
    environment: 'jsdom',
    // 给 jsdom 一个 origin：否则部分 web 全局初始化需要窗口 origin。
    environmentOptions: { jsdom: { url: 'http://localhost/' } },
    include: ['src/**/*.test.ts'],
  },
});
