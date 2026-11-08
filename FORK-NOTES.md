# Fork mapo80/go-openai

Fork of `github.com/meguminnnnnnnnn/go-openai` for ARC (`arc-workspace`). The module path is unchanged; ARC uses it with `replace github.com/meguminnnnnnnnn/go-openai => github.com/mapo80/go-openai <tag>`.

- **Upstream base:** tag `v0.1.6`, commit `a92c8d868a5db59bb6e0e40a2230c417edb1bd04`.
- **License:** unchanged (`LICENSE`, Apache 2.0 as upstream).
- **Branch of the changes:** `arc/json-v2`, merged into `master`.
- **Go:** `go 1.27` (`encoding/json/v2` and `encoding/json/jsontext` from the standard library).

## Changes

| commit | change |
|---|---|
| `f0c8c05` | Each SSE chunk is decoded once with `encoding/json/v2` and the semantics of `encoding/json` (`DefaultOptionsV1`): the delta reads its members as tokens, declared members into their fields, the others kept as extra fields (own copies). Before: `encoding/json` scan, `sonic` decode of the response, `encoding/json` + `sonic` map again for each delta. SSE `data:` prefix cut without regular expressions. Extra fields of usage and messages read as tokens. `bytedance/sonic` is no longer a dependency. |
| `781fff2` | The request is written with `encoding/json/v2` (messages through `MarshalJSONTo`, no bytes of their own) and its extra fields merged by `MergePatch` on `jsontext`, in place, opening only the objects the patch reaches, with the bytes `evanphx/json-patch` v0.5.2 gave. `json-patch` remains a test-only reference. |

## Measures (8 November 2026)

Replay of the 67 captured Callistos model calls (`blocco4/cattura-live`: 6 651 SSE chunks, 175 with tool calls, 67 with usage and finish reason) through `eino-ext/components/model/openai` v0.1.13 → `eino-ext/libs/acl/openai` v0.1.17 → this SDK, warm second round:

| | upstream v0.1.6 | fork |
|---|---|---|
| bytes allocated per model call (request + stream) | 2 814 334 | 237 413 |
| bytes allocated per chunk | 28 350 | 2 391 |
| chunk messages (content, tool calls, reasoning, usage, finish reason, extra) | — | identical, 6 651 / 6 651 |
| request bodies re-marshalled from the captures | — | identical bytes, 67 / 67 |

Tests: `TestStreamDeltaDecodesAsBeforeInOnePass` (against `encoding/json` + extra-field map: case variants, duplicates, nulls, invalid UTF-8, errors), `TestStreamChunkKeepsRawBodyAndOwnExtras`, `TestDataPayloadMatchesTheSSEPrefix`, `TestMergePatchMatchesJSONPatch` (against `evanphx/json-patch`); upstream suite green.
