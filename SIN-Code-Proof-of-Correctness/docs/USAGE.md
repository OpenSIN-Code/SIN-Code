# Usage — POC

The package installs the `poc` command.

## `poc suggest <module_path> <function_name>`

Infer candidate properties and print a runnable Hypothesis test.

```bash
poc suggest mymodule.py normalize
```

## `poc verify <module_path> <function_name>`

Run runtime verification and report a per-property result.

```bash
poc verify mymodule.py normalize --max-examples 100
```

Example output:

```json
{
  "pure": "PASS",
  "idempotent": "PASS",
  "non_negative_output": "FAIL"
}
```

Possible results: `PASS`, `FAIL`, `SKIPPED` (e.g. a property that needs an
inverse function), or `ERROR: <Type>`.

## `poc from-intent "<intent>" <function_name>`

Compile a natural-language intent into a spec test.

```bash
poc from-intent "returns a non-negative integer" compute
```

## Python API

```python
from sin_code_poc import PropertyGenerator, RuntimeVerifier

sig = RuntimeVerifier.extract_signature(my_fn)
props = PropertyGenerator().suggest(sig)
results = RuntimeVerifier().verify_function(my_fn, props)
```

## Built-in properties

| Property | Meaning |
|----------|---------|
| `pure` | same input → same output |
| `idempotent` | `f(f(x)) == f(x)` |
| `reversible` | an inverse exists (`SKIPPED` unless provided) |
| `monotonic` | `x <= y ⇒ f(x) <= f(y)` |
| `non_negative_output` | output `>= 0` |
| `length_preserving` | `len(f(x)) == len(x)` |
| `sorted_output` | output is sorted |
