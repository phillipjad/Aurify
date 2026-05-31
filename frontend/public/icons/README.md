# PWA icons

These were generated with
[`@vite-pwa/assets-generator`](https://vite-pwa-org.netlify.app/assets-generator/)
and are referenced by the web manifest in `../../vite.config.ts` and by
`../../index.html`:

- `pwa-64x64.png`, `pwa-192x192.png`, `pwa-512x512.png` — manifest icons
- `maskable-icon-512x512.png` — maskable variant (`purpose: maskable`)
- `apple-touch-icon-180x180.png` — iOS home-screen icon
- `favicon.ico` — legacy favicon

To regenerate from `../favicon.svg`:

```bash
pnpm dlx @vite-pwa/assets-generator --preset minimal-2023 ../favicon.svg
```

If you change the preset/output names, update the `icons` array in
`vite.config.ts` and the `<link>` tags in `index.html` to match.
