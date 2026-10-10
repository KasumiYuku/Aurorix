import { defineConfig } from 'vite'
import solid from 'vite-plugin-solid'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  base: '/admin/',
  plugins: [solid(), tailwindcss()],
  build: {
    outDir: '../lib/admin/dist',
    emptyOutDir: true,
    target: 'es2022',
    assetsInlineLimit: 4096,
  },
  server: {
    port: 5173,
    proxy: {
      '/admin/api': {
        target: 'http://127.0.0.1:4514',
        changeOrigin: true,
      },
    },
  },
})
