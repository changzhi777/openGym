// src/test/setup-local-storage.mjs — 修复 Node >= 24 与 happy-dom 的 localStorage 冲突
//
// 现象(2026-10-21 实测,Node 26.7/vitest 4.1.10/happy-dom 20.11):
//   Node >= 24 挂了实验性全局 localStorage — 值为 undefined(除非 --localstorage-file)。
//   组件测试用 `// @vitest-environment happy-dom` 切环境后,vitest 把 happy-dom 的
//   window 展开到全局,但已被 Node 占位的 `localStorage` 没有被覆盖 →
//   `localStorage.getItem is not a function` → frontend 565 个用例红。
//   Node 22(上游 CI 版本)没有这个全局,所以上游看不见。
//
// 修法:setup 在每个测试环境里兜底 — 优先用 happy-dom window 自带的(保真),
// 拿不到再退内存实现。仅当当前全局缺失/不可用时才动手,不动正常环境。
const current = globalThis.localStorage
const usable = current && typeof current.getItem === 'function'
if (!usable) {
  const fromWindow = globalThis.window && globalThis.window.localStorage
  const impl = (fromWindow && typeof fromWindow.getItem === 'function')
    ? fromWindow
    : (() => {
        const store = new Map()
        return {
          getItem: k => (store.has(k) ? store.get(k) : null),
          setItem: (k, v) => store.set(k, String(v)),
          removeItem: k => store.delete(k),
          clear: () => store.clear(),
          key: i => [...store.keys()][i] ?? null,
          get length() { return store.size },
        }
      })()
  Object.defineProperty(globalThis, 'localStorage', {
    value: impl, configurable: true, writable: true,
  })
}
