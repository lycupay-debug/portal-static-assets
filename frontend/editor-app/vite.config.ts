import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  root: fileURLToPath(new URL('.', import.meta.url)),
  base: '/editor/',
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('../src', import.meta.url)),
    },
  },
  build: {
    outDir: fileURLToPath(new URL('../dist-editor', import.meta.url)),
    emptyOutDir: true,
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('/node_modules/@tiptap/') || id.includes('/node_modules/prosemirror-')) return 'editor-engine'
          if (id.includes('/node_modules/@vue/') || id.includes('/node_modules/vue/')) return 'vue-runtime'
        },
      },
    },
  },
  server: {
    port: 5174,
    proxy: {
      '/api': { target: 'http://127.0.0.1:8081', changeOrigin: true, rewrite: path => path.replace(/^\/api/, '') },
    },
  },
})
