# Responsive and accessibility verification

The customer storefront is designed for keyboard, touch, and pointer input from a 320 px-wide mobile viewport through desktop layouts.

## Implemented behavior

- The archive, product, checkout, and confirmation flows scroll naturally on mobile instead of being locked to the viewport.
- Archive rows and product options use native buttons with visible focus styles and keyboard activation.
- Sign-in remains available on mobile. Its modal uses dialog semantics, traps focus, closes with Escape, and restores focus to its trigger.
- Form controls have programmatic labels, autocomplete hints, required state, and live error announcements.
- Status, timer, and error updates expose appropriate live-region semantics without repeatedly announcing decorative animation.
- Motion follows `prefers-reduced-motion`; global CSS also removes nonessential transition and animation duration.
- Muted text tokens meet WCAG AA contrast against the black storefront background.

## Automated checks

```bash
npm run test:a11y
```

The Playwright suite runs in Desktop Chrome and a Pixel 5 profile. It checks:

- WCAG 2 A/AA and 2.1 A/AA rules with `@axe-core/playwright`;
- keyboard activation of a drop;
- focus trapping and focus restoration in the sign-in dialog;
- horizontal overflow on the archive and product page;
- the interface with reduced motion enabled.

Serious and critical axe violations fail CI. Automated checks complement, rather than replace, manual screen-reader and zoom testing.

## Lighthouse baseline

Measured on 21 July 2026 against a local Next.js production build, using Lighthouse 13.4.0 with its default mobile throttling. The performance values are medians from three consecutive runs:

| Category | Score |
| --- | ---: |
| Performance | 89 |
| Accessibility | 100 |
| Best Practices | 100 |
| SEO | 100 |

The median largest contentful paint was `2.12 s` (individual runs: `2.514 s`, `2.120 s`, and `1.946 s`), median total blocking time was `0.40 s`, and cumulative layout shift was `0`. Performance is expected to vary on local Windows hardware; the accessibility, best-practices, and SEO baselines are deterministic. A hosted Lighthouse run should be captured after issue #19 receives its public URL.

Reproduce the audit against a production server:

```bash
npx lighthouse@13.4.0 http://127.0.0.1:3000/drops \
  --only-categories=performance,accessibility,best-practices,seo \
  --output=html \
  --output-path=./lighthouse-report.html
```

The generated report is intentionally ignored and should not be committed.
