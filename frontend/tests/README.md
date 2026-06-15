# Frontend Tests

The frontend uses Node's built-in test runner.

```sh
npm test
```

Use `node:test`, `node:assert/strict`, and `fast-check` directly. Do not use `test.prop`; Vitest/Jest bindings are not installed.

Property test pattern:

```ts
import assert from 'node:assert/strict'
import { it } from 'node:test'
import fc from 'fast-check'

it('normalizes valid ids', () => {
  fc.assert(
    fc.property(fc.stringMatching(/^\d{5,20}$/), (raw) => {
      assert.match(raw, /^\d{5,20}$/)
    }),
    { numRuns: 100 },
  )
})
```

Keep generators constrained instead of filtering heavily with `fc.pre`.
