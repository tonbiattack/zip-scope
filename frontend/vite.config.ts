import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// The Wails asset server serves the built bundle from frontend/dist, and the
// image handler answers /img/<id>. No proxy is needed in development because
// the Go side is the only origin.
export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'chrome110',
    sourcemap: false,
  },
})
