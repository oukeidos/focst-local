# FoCST Local

`focst-local` is a CLI subtitle translator for local LLM runs through
llama.cpp's OpenAI-compatible server.

The MVP translation path is local-first:

- Backend: `llama-server`
- Default endpoint: `http://127.0.0.1:8080/v1`
- Default model: `gemma-4-26b-a4b-qat-q4_0`
- Translation output contract: one translated `text` value for each input
  segment ID
- Optional `names` workflow: OpenAI-based character name mapping

## Build

```bash
go build -o focst-local ./cmd/focst-local
```

To install into `$(go env GOPATH)/bin`:

```bash
go install -buildvcs=false ./cmd/focst-local
```

## Run

The default workflow is still externally managed llama.cpp: start
`llama-server` yourself, then point `focst-local` at it.

The Gemma 4 examples below include `--reasoning off` because that is the tested
local translation setup. `focst-local` does not add this flag automatically; pass
it explicitly when your model/server needs it.

```bash
llama-server \
  --model /path/to/gemma-4-26b-a4b-qat-q4_0.gguf \
  --alias gemma-4-26b-a4b-qat-q4_0 \
  --reasoning off \
  --parallel 1 \
  --ctx-size 16384
```

```bash
./focst-local translate input.srt output.srt \
  --llama-base-url http://127.0.0.1:8080/v1 \
  --model gemma-4-26b-a4b-qat-q4_0 \
  --source en \
  --target ko \
  --log-file run.jsonl \
  --yes
```

The root command also translates:

```bash
./focst-local input.srt output.srt --source en --target ko --yes
```

`focst-local` can also start llama.cpp when both paths are provided
explicitly. It does not guess model paths from LM Studio caches, workspace
symlinks, or local experiment directories.

```bash
./focst-local translate input.srt output.srt \
  --llama-server-mode start \
  --llama-server-bin /path/to/llama-server \
  --model-path /path/to/gemma-4-26b-a4b-qat-q4_0.gguf \
  --model gemma-4-26b-a4b-qat-q4_0 \
  --ctx-size 16384 \
  --parallel 1 \
  --llama-arg --reasoning \
  --llama-arg off \
  --source en \
  --target ko \
  --yes
```

In `start` mode, the managed server is stopped after the command unless
`--keep-llama-server` is set.

## User Config

User config is a convenience layer for local paths and llama.cpp defaults. It
is not a built-in model profile system.

```bash
./focst-local config path
./focst-local config set llama-server-bin /path/to/llama-server
./focst-local config set model-path /path/to/gemma-4-26b-a4b-qat-q4_0.gguf
./focst-local config set model gemma-4-26b-a4b-qat-q4_0
./focst-local config set ctx-size 16384
./focst-local config set parallel 1
./focst-local config add-arg --reasoning
./focst-local config add-arg off
./focst-local config show
```

After that, managed start can be shorter:

```bash
./focst-local translate input.srt output.srt \
  --llama-server-mode start \
  --source en \
  --target ko \
  --yes
```

Config resolution uses this order:

```text
CLI flag > environment variable > user config > built-in default
```

Supported environment variables are `FOCST_LLAMA_SERVER_BIN`,
`FOCST_LLAMA_MODEL_PATH`, `FOCST_LLAMA_CTX_SIZE`, and
`FOCST_LLAMA_PARALLEL`.

## Translation Contract

The product keeps FoCST's chunking, context windows, preprocessing,
postprocessing, recovery logs, and name mapping support. The local translation
contract is intentionally simpler than upstream FoCST:

- The model receives chunked target segments plus surrounding context.
- Context is used only to resolve meaning.
- Each model-facing segment carries a single `source_text` field. Physical
  subtitle lines are joined with spaces and whitespace is normalized before
  the request is sent.
- The model returns JSON shaped as
  `{"translations":[{"id":1,"source_text":"...","text":"..."}]}`.
- Target IDs must match exactly: no missing, duplicate, or extra IDs.
- `source_text` must exactly echo the same normalized target segment text that
  appeared in the request.
- Each segment is saved by replacing the source segment text with the returned
  translated text.

Line wrapping is not a model contract in the MVP. If stricter wrapping is needed
later, it should be a deterministic postprocess, not an LLM formatting burden.

## Local Runtime Flags

- `--llama-base-url`: OpenAI-compatible llama.cpp base URL.
- `--llama-server-mode`: `external` or `start`. Default: `external`.
- `--llama-server-bin`: llama.cpp server executable path for `start` mode.
- `--model-path`: GGUF model file path for `start` mode.
- `--model`: local model name exposed by the running server.
- `--ctx-size`: llama.cpp context size for `start` mode. Default: `16384`.
- `--parallel`: llama.cpp parallel slots for `start` mode. Default: `1`.
- `--threads`: optional llama.cpp thread count for `start` mode.
- `--cache-ram`: optional llama.cpp cache RAM setting for `start` mode.
- `--llama-arg`: extra llama.cpp argv token for advanced settings.
- `--keep-llama-server`: leave a managed server running after the command.
- `--llama-load-timeout`: managed server readiness timeout.
- `--llama-log-file`: managed server stdout/stderr log path.
- `--max-tokens`: maximum generated tokens per request. Default: `8192`.
- `--translation-timeout`: timeout per translation request. Default: `0`
  means unlimited; use values such as `30m` only when you want a hard cap.
- `--chunk-size`: target segments per request. Default: `100`.
- `--context-size`: surrounding context segments before and after each chunk.
- `--concurrency`: concurrent chunk requests. Keep this at `1` for a single
  local consumer GPU unless you intentionally run multiple llama.cpp slots.
- `--log-file`: machine-readable JSONL log path.
- `--names`: character name mapping JSON file generated by `names`.

Every translation run prints wall time and token usage when the server returns
usage metadata. Chunk completion events include per-chunk duration and token
counts in JSONL logs.

For the tested Gemma 4 26B QAT model, use `llama-server --parallel 1`,
`--ctx-size 16384`, and `--reasoning off`. `--ctx-size 8192` handled
`chunk-size=100` in the sample run with `--max-tokens 6144`, but the product
default keeps FoCST's 100-segment chunking and raises the generation budget to
`--max-tokens 8192`, so 16384 server context is the practical default target.
It is a recommendation, not a hard requirement: smaller context can work if
chunk size, context size, or output budget is reduced.

## Names

The `names` command remains OpenAI-based and separate from translation:

```bash
./focst-local env setup --service openai
./focst-local names --title "Example Title" --source English --target Korean names.json
./focst-local translate input.srt output.srt --names names.json --yes
```

Use `--allow-env` or `--env-only` with `names` if you want to read
`OPENAI_API_KEY` from the environment. Translation itself does not use an API
key.

## Recovery

Failed or partially completed translation runs write recovery logs next to the
output file. Repair uses the local llama.cpp backend:

```bash
./focst-local repair output_recovery.json --llama-base-url http://127.0.0.1:8080/v1
```

## Repository

https://github.com/oukeidos/focst-local
