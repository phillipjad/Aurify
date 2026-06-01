import { defineConfig } from 'vite-plus'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import { VitePWA } from 'vite-plugin-pwa'
import path from 'node:path'

// Single Vite+ config: drives dev, build, format (`fmt`), and tests (`test`).
// Vite+ reads Oxfmt + Vitest options from here — not from .oxfmtrc.json or a
// standalone vitest.config.ts.
export default defineConfig({
  plugins: [
    // Generates src/routeTree.gen.ts from the files in src/routes.
    tanstackRouter({ target: 'react', autoCodeSplitting: true }),
    react(),
    tailwindcss(),
    VitePWA({
      registerType: 'autoUpdate',
      includeAssets: ['favicon.svg', 'favicon.ico', 'icons/apple-touch-icon-180x180.png'],
      manifest: {
        name: 'Aurify',
        short_name: 'Aurify',
        description: 'Generate album/playlist covers that capture the vibe of your music.',
        theme_color: '#0b0b0f',
        background_color: '#0b0b0f',
        display: 'standalone',
        start_url: '/',
        icons: [
          { src: '/icons/pwa-64x64.png', sizes: '64x64', type: 'image/png' },
          {
            src: '/icons/pwa-192x192.png',
            sizes: '192x192',
            type: 'image/png',
          },
          {
            src: '/icons/pwa-512x512.png',
            sizes: '512x512',
            type: 'image/png',
          },
          {
            src: '/icons/maskable-icon-512x512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable',
          },
        ],
      },
    }),
  ],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  server: {
    port: 5173,
    proxy: {
      // Proxy API calls to the Go backend during development.
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
  fmt: {
    // Generated/build artifacts — keep Oxfmt off them (was .prettierignore).
    ignorePatterns: ['dist/**', 'pnpm-lock.yaml', 'src/routeTree.gen.ts', 'src/lib/api/schema.ts'],
    singleQuote: true,
    semi: false,
    trailingComma: 'all',
    printWidth: 120,
  },
  lint: {
    // Oxlint ignores (was .oxlintrc.json) — keep generated files unlinted.
    ignorePatterns: ['dist/**', 'src/routeTree.gen.ts', 'src/lib/api/schema.ts'],
    options: {
      typeAware: true,
      typeCheck: true,
    },
  },
  test: {
    environment: 'jsdom',
    globals: false,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
