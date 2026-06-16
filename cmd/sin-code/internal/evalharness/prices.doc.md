# evalharness/prices.go — per-arm self-pricing

The comparator's matrix column `median_usd` is computed from a
prompt + completion token count and a price-book entry. The
price book is in this file because:

- it is too small (6 entries) to warrant a separate package;
- it MUST be inline so a binary diff at the same git SHA
  produces the same USD numbers across CI runs (mandate M2
  byte-stability);
- the comparator MUST NOT make network calls (mandate M2).

## Adding a model

Append to `Prices` with a free-form key. Pick the same key the
caller sets in `Arm.PricingName` (or `--model-pricing`). Unknown
keys produce a warning in `CompareReport.Warnings` and zero USD.

## Cost formula

```
usd(prompt, completion) = prompt/1000 * PromptPer1k
                        + completion/1000 * CompletionPer1k
```

Rounded to 6 decimals so repeated rounding does not drift a
snapshot.

## Round-trip

`Cost` is a pure function. Tests assert idempotency on identical
(token, price) inputs.

## Related files

- `comparator.go` — wires `Arm.PricingName` into `Cost`.
- `snapshot.go` — writes `median_usd` per row.
