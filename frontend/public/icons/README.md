# PWA icons

Add the PNG icons referenced by the web manifest in `vite.config.ts`:

- `pwa-192.png` — 192×192
- `pwa-512.png` — 512×512
- `pwa-512-maskable.png` — 512×512, with safe-area padding (`purpose: maskable`)

You can generate them from `../favicon.svg` with a tool such as
[`@vite-pwa/assets-generator`](https://vite-pwa-org.netlify.app/assets-generator/):

```bash
pnpm dlx @vite-pwa/assets-generator --preset minimal-2023 ../favicon.svg
```
