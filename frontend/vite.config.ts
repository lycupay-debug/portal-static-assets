import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'

export default defineConfig({
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    // The editor is intentionally loaded asynchronously; warn only above 3 MB.
    chunkSizeWarningLimit: 3000,
  },
})
