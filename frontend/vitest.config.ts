import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// Vitest needs its own config so the `test` block is typed; the build config
// stays free of test-only options.
export default defineConfig({
  plugins: [vue()],
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
    globals: false,
  },
})
