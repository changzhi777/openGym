// src/test/setup-local-storage.mjs — keep localStorage working under Node >= 24 with happy-dom
//
// Seen with Node 26.7 / vitest 4.1.10 / happy-dom 20.11:
//   Node >= 24 installs an experimental global localStorage whose value is undefined (unless --localstorage-file).
//   A component test that opts into `// @vitest-environment happy-dom` gets the window
//   globals spread over, but Node's own `localStorage` placeholder is not overwritten, so
//   `localStorage.getItem is not a function` - 565 frontend tests fail.
//   Node 22 (the version upstream CI runs) has no such global, so CI never sees it.
//
// Fix: the setup backstops every test environment - prefer the happy-dom window's own
// implementation (faithful), fall back to an in-memory one. It only acts when the current global is missing or unusable.
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
