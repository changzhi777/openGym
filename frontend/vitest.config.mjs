// vitest.config.mjs — deliberately not inlined into vite.config.js, to keep that file close to upstream.
// Its only job is to load the setup that restores localStorage under Node >= 24
// (see src/test/setup-local-storage.mjs for the why).
// mergeConfig keeps everything vite.config.js already sets (fs.allow, ...).
import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config.js'

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: { setupFiles: ['./src/test/setup-local-storage.mjs'] },
  }),
)
