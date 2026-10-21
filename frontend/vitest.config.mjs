// vitest.config.mjs — fork-only：不放进 vite.config.js，避免与上游 rebase 冲突。
// 唯一职责：挂 setup，修复 Node >= 24 下实验性全局 localStorage(undefined)
// 遮蔽 happy-dom 注入的问题(详见 src/test/setup-local-storage.mjs 头注释)。
// mergeConfig 保留 vite.config.js 的既有配置(fs.allow 等)。
import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config.js'

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: { setupFiles: ['./src/test/setup-local-storage.mjs'] },
  }),
)
