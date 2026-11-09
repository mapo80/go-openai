# Fork mapo80/go-openai

Fork of `github.com/meguminnnnnnnnn/go-openai` for ARC (`arc-workspace`). The module path is unchanged; ARC uses it with `replace github.com/meguminnnnnnnnn/go-openai => github.com/mapo80/go-openai <tag>`.

- **Upstream base:** tag `v0.1.6`, commit `a92c8d868a5db59bb6e0e40a2230c417edb1bd04`.
- **License:** unchanged (`LICENSE`, Apache 2.0 as upstream).
- **Branch of the changes:** `arc/json-v2`, merged into `master` (`v0.1.6-arc.1`); `arc/request-once` (`v0.1.6-arc.2`).
- **Go:** `go 1.27` (`encoding/json/v2` and `encoding/json/jsontext` from the standard library).

## Changes

| commit | change |
|---|---|
| `f0c8c05` | Each SSE chunk is decoded once with `encoding/json/v2` and the semantics of `encoding/json` (`DefaultOptionsV1`): the delta reads its members as tokens, declared members into their fields, the others kept as extra fields (own copies). Before: `encoding/json` scan, `sonic` decode of the response, `encoding/json` + `sonic` map again for each delta. SSE `data:` prefix cut without regular expressions. Extra fields of usage and messages read as tokens. `bytedance/sonic` is no longer a dependency. |
| `781fff2` | The request is written with `encoding/json/v2` (messages through `MarshalJSONTo`, no bytes of their own) and its extra fields merged by `MergePatch` on `jsontext`, in place, opening only the objects the patch reaches, with the bytes `evanphx/json-patch` v0.5.2 gave. `json-patch` remains a test-only reference. |
| `f7b4984` | A request with extra fields writes its messages and tools once. Before, the whole request was marshalled and then merged with the extra fields into a second document, so nearly all of the body was written twice (and the merge buffer grew again while flushing). `ChatCompletionRequest.SplitMembers` hands the marshaller the messages and the tools the extra fields do not name; the rest of the request is merged as before (`mergePatch`), and the root of the merge writes those members straight into the encoder of the request. Members the extra fields name stay in the merged document. Errors keep their precedence: on failure the whole body is marshalled again and its error is reported first. |

## Measures (8 November 2026)

Replay of the 67 captured Callistos model calls (`blocco4/cattura-live`: 6 651 SSE chunks, 175 with tool calls, 67 with usage and finish reason) through `eino-ext/components/model/openai` v0.1.13 → `eino-ext/libs/acl/openai` v0.1.17 → this SDK, warm second round:

| | upstream v0.1.6 | fork |
|---|---|---|
| bytes allocated per model call (request + stream) | 2 814 334 | 237 413 |
| bytes allocated per chunk | 28 350 | 2 391 |
| chunk messages (content, tool calls, reasoning, usage, finish reason, extra) | — | identical, 6 651 / 6 651 |
| request bodies re-marshalled from the captures | — | identical bytes, 67 / 67 |

Tests: `TestStreamDeltaDecodesAsBeforeInOnePass` (against `encoding/json` + extra-field map: case variants, duplicates, nulls, invalid UTF-8, errors), `TestStreamChunkKeepsRawBodyAndOwnExtras`, `TestDataPayloadMatchesTheSSEPrefix`, `TestMergePatchMatchesJSONPatch` (against `evanphx/json-patch`); upstream suite green.

## Measures (9 November 2026, `v0.1.6-arc.2`)

`BenchmarkRequestBodyWithExtraFields` (a streamed request of 181 521 B, the messages and one tool nearly all of it, with `enable_thinking`), 5 runs:

| | `v0.1.6-arc.1` | `v0.1.6-arc.2` |
|---|---|---|
| bytes allocated per body | 957 382 – 1 005 200 | 212 345 – 213 999 |
| time per body | 403 – 433 µs | 148 – 151 µs |

ARC, Callistos conversation replayed by `TestCallistosConversationTurnCost` (warm runtime, exact profiles of turn 2, 5 runs per side): `ports/model.(*Factory).Stream` allocates 397.9 KB less per turn (the merge's copy of the 4 request bodies, 129 902 B of prompts); the 10 requests to the model are equal.

Tests: `TestRequestBodyMatchesArc1` (6000 random requests and extra fields against the code of `v0.1.6-arc.1`, kept as a test oracle: bytes and errors), `TestRequestBodyErrorsMatchArc1` (error precedence), `TestClientSendsTheArc1RequestBody` (bodies the client sends, streaming or not, with and without a body modifier), `TestRequestBodyWithExtraFieldsIsWrittenOnce` (red before: 724 860 B more with the extra fields than without, for a 181 521 B body); `TestMergePatchMatchesJSONPatch` unchanged in cases and expectations; suite green, also with `-race`.
