# Storefront performance

## Product image baseline

Measured on 22 July 2026 from the versioned product assets and the rendered gallery structure:

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Product source assets | 2.62 MiB | 161.6 KiB | -94.0% |
| Full-size gallery image layers at first render | 6 | 1 | -83.3% |
| Full-size archive preview image layers | 2 | 1 | -50.0% |

The original files used a `.jpg` extension but contained transparent PNG data. They were converted to transparent AVIF at their native dimensions with quality 72 and 4:4:4 chroma sampling. Visual inspection confirmed that the garment edges, hardware, texture, and transparency remain intact.

Next.js still produces responsive variants at request time. The archive and initial product image use high-priority eager loading with accurate viewport `sizes`; gallery thumbnails and non-initial active slides remain lazy. Only the active full-size gallery image is mounted, so hidden slides no longer consume full-size decode surfaces.

A post-change local Lighthouse 13.4.0 mobile validation of the archive scored Performance 95, Accessibility 100, Best Practices 100, and SEO 100, with LCP `2.392 s`, TBT `0.189 s`, and CLS `0`. The mobile archive intentionally transfers no product preview because that desktop-only panel is not rendered at the mobile breakpoint.

## GPU and motion budget

- Fullscreen duplicate image blurs were replaced by static gradients.
- Runtime SVG turbulence was replaced by lightweight CSS texture gradients.
- The ticker, spinners, and status pulses use CSS animations covered by the global reduced-motion rule.
- The ticker and CRT vignette are disabled on reduced-motion devices; the vignette is also removed on small screens.
- The critical-stock fullscreen glow is static instead of running an infinite opacity animation.

## Reproduce

```bash
npm run build
npx playwright test e2e/pointer-performance.spec.ts e2e/server-rendering.spec.ts
npx lighthouse@13.4.0 http://127.0.0.1:3000/drops \
  --only-categories=performance,accessibility,best-practices,seo
```

Use `E2E_API_PORT` and `E2E_WEB_PORT` to run Playwright alongside an existing local development server.
