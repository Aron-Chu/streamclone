# Frontend tests

Frontend tests run on Node's built-in test runner (no Vitest/Jest in this repo):

```bash
npm test   # node --experimental-strip-types --test tests/*.test.ts
```

Tests import `test`/`it` from `node:test` and assertions from `node:assert/strict`
(see `playbackMath.test.ts`).

## Property-based tests (`fast-check`)

Property tests use [`fast-check`](https://github.com/dubzzz/fast-check) (a `devDependency`).

> **There is NO `test.prop([...], cb)` macro in this repo.** The `@fast-check/vitest`
> / `@fast-check/jest` `test.prop` / `it.prop` bindings are not installed and the
> Node runner does not provide them. Do not write `test.prop([...], (x) => { ... })`.

Instead, wrap `fc.assert(fc.property(...))` inside a standard runner `it(...)` (or
`test(...)`) block. Use `fc.asyncProperty` for async properties:

```ts
import assert from 'node:assert/strict'
import { it } from 'node:test'
import fc from 'fast-check'
import { normalizeVodId } from '../src/utils/vodId.ts'

// Feature: moment-timeline, Property 1: VOD Identifier Normalization Round-Trip
it('normalizes valid vod ids idempotently', () => {
  fc.assert(
    fc.property(fc.stringMatching(/^\d{5,20}$/), (raw) => {
      const once = normalizeVodId(raw)
      assert.match(once!, /^\d{5,20}$/)
      assert.equal(normalizeVodId(once!), once) // idempotent
    }),
    { numRuns: 100 },
  )
})

// Async variant
it('async property example', () => {
  return fc.assert(
    fc.asyncProperty(fc.integer(), async (n) => {
      const result = await someAsyncFn(n)
      assert.ok(result >= 0)
    }),
    { numRuns: 100 },
  )
})
```

### Conventions for moment-timeline property tests

- Run **≥100 iterations** (`{ numRuns: 100 }` or higher).
- Tag each property test with `// Feature: moment-timeline, Property {N}: {title}`.
- Link the requirement in the test description or a comment as
  `**Validates: Requirements X.Y**`.
- Prefer smart generators that constrain inputs to the valid space rather than
  filtering with `fc.pre`.
