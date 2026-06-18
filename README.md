# FoCST Local

`focst-local` is a CLI subtitle translator for local LLM runs through
llama.cpp's OpenAI-compatible server.

It has only been tested with Gemma 4 26B A4B and Gemma 4 31B. Other local
models may work, but they are not validated and are not guaranteed to preserve
the subtitle translation contract.

The main failure mode for local subtitle translation is segment drift: the model
may translate the right general scene, but place content from one subtitle
segment into another segment's output. `focst-local` reduces this by requiring
each translated item to echo the original source text together with the
translation for the same segment ID, making it much harder for nearby subtitle
content to slide into the wrong output slot.

## Build

Clone the repository first:

```bash
git clone https://github.com/oukeidos/focst-local.git
cd focst-local
```

Build the CLI:

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

By default, `focst-local` expects an OpenAI-compatible `llama-server` endpoint at
`http://127.0.0.1:8080/v1` and uses the model alias
`gemma-4-26b-a4b-qat-q4_0`.

```bash
llama-server \
  --model /path/to/gemma-4-26b-a4b-qat-q4_0.gguf \
  --alias gemma-4-26b-a4b-qat-q4_0 \
  --reasoning off \
  --parallel 1 \
  --ctx-size 16384
```

Run Gemma 4 with reasoning disabled. The tested token and context settings are
intended for direct translation output, not for additional reasoning tokens, and
reasoning-enabled runs may fail or produce unusable responses.

```bash
./focst-local translate input.srt output.srt \
  --llama-base-url http://127.0.0.1:8080/v1 \
  --model gemma-4-26b-a4b-qat-q4_0 \
  --source en \
  --target ko
```

Use `--source` and `--target` for language codes such as `ja`, `en`, and `ko`.

The root command also translates:

```bash
./focst-local input.srt output.srt --source en --target ko
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
  --target ko
```

## Translation Behavior

`focst-local` keeps the practical parts of
[FoCST](https://github.com/oukeidos/focst)'s subtitle pipeline while using a
local model for translation. By default it translates multiple subtitle segments
at a time as a chunk, includes nearby preceding and following subtitles as
context for that chunk, applies preprocessing before model requests, and applies
deterministic postprocessing before writing the output file.

Preprocessing and postprocessing include subtitle cleanup and normalization
steps that are useful for ordinary runs. They can be adjusted or disabled with
flags when you need closer control over input handling or output formatting.
Run `./focst-local --help` or `./focst-local translate --help` for the current
translation, preprocessing, postprocessing, chunking, logging, and recovery
options.

## Experimental Local Quality Passes

`focst-local` includes three optional local quality passes: glossary generation,
phrase anchors, and post-translation polish. For practical use, the glossary
pass is recommended because it often improves name and term consistency. Phrase
anchors and post-polish are purely experimental: they may occasionally help, but
current testing does not show reliable quality improvement and they can make
translations worse.

Recommended use:

- Start with `--auto-glossary` for important translations.
- Use phrase anchors or post-polish only for experiments, debugging, or manual
  comparison runs.
- Save and inspect artifacts before trusting output from any experimental pass.

### Local Glossary

`focst-local` can generate a local glossary with the same local model used for
translation, then inject that glossary into the translation prompt. This helps
keep names, organizations, recurring proper nouns, and specialized terms more
consistent.

Use `--auto-glossary` to generate and apply a glossary in one run:

```bash
./focst-local translate input.srt output.srt \
  --source en \
  --target ko \
  --auto-glossary
```

Add `--save-glossary terms.glossary.json` when you want to keep the generated
glossary artifact.

To inspect or edit the glossary first, split the workflow:

```bash
./focst-local glossary extract input.srt terms.glossary.json \
  --source en \
  --target ko

./focst-local translate input.srt output.srt \
  --source en \
  --target ko \
  --glossary-file terms.glossary.json
```

The default glossary pass runs extraction 3 times per glossary window and merges
the results by vote. Use `--glossary-runs 10` when you want a more stable
glossary artifact before translation.

Known limits:

- Generated glossaries are stochastic; save and inspect them for important jobs.
- More runs reduce random variation, but cannot fix systematically wrong terms.
- The glossary mainly improves terminology consistency, not general sentence
  translation quality.
- Explicit `--names` mappings take priority over generated glossary entries.

### Local Phrase Anchors

`focst-local` can also generate phrase anchors with the local model. Phrase
anchors are soft phrase-level guidance for local ambiguity, idioms, wordplay,
callbacks, and short scene-specific expressions. They do not replace the
glossary; glossary and `--names` mappings remain the stronger signal for names
and stable terms.

Use `--auto-phrase-anchors` to generate and apply them in one run:

```bash
./focst-local translate input.srt output.srt \
  --source ja \
  --target ko \
  --auto-phrase-anchors
```

Add `--save-phrase-anchors anchors.json` to keep the generated artifact, or
split extraction and translation when you want to inspect it first:

```bash
./focst-local phrase-anchors extract input.srt anchors.json --source ja --target ko
./focst-local translate input.srt output.srt --source ja --target ko --phrase-anchors-file anchors.json
```

Known limits: phrase anchors are stochastic, high-cost, and experimental. They
can improve difficult local phrasing, but wrong anchors can also hurt quality;
save and inspect artifacts for important jobs.

### Post-Translation Polish

`focst-local` can run an optional local polish pass after translation. It asks
the local model to improve target-language phrasing after the main translation
has already completed. This is experimental and should be treated as a
comparison tool, not a guaranteed quality upgrade.

Post-polish has explicit profiles:

- `segment-local`: prefers local segment fidelity. This is the default for
  `--post-polish` and is the safer profile when each subtitle ID should stay
  close to its own source line.
- `chunk-flow`: prefers cross-segment sentence flow. It can help when one
  sentence often spans several subtitle IDs, but it is more aggressive and may
  over-edit.
- `legacy`: uses the older correction-only two-pass polish implementation.

Use `--post-polish`:

```bash
./focst-local translate input.srt output.srt \
  --source ja \
  --target ko \
  --post-polish
```

Use `--post-polish-profile chunk-flow` or `--post-polish-profile legacy` when
you want a non-default profile.

Add `--save-polish-corrections corrections.polish.json` when you want to keep
the polish artifact.

To inspect or reuse an existing translation first, split the workflow:

```bash
./focst-local translate input.srt draft.srt \
  --source ja \
  --target ko

./focst-local polish input.srt draft.srt polished.srt \
  --source ja \
  --target ko
```

Post-polish works without glossary or names. When glossary or names mappings
are available, they are used as a protective guard so polish candidates do not
remove protected renderings.

Known limits: post-polish can still make bad edits. In particular, subtitles
whose meaning is spread across multiple IDs can be difficult to polish safely.
Save and inspect outputs for important jobs.

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

`focst-local` includes an optional OpenAI-based names workflow that generates a
character and proper-name mapping file, then applies that mapping during
translation with `--names`. API key setup and API-key-related options exist for
this names workflow only; subtitle translation itself is still performed locally
through llama.cpp.

Run `./focst-local names --help` for the current options.

## Recovery

`focst-local` includes a repair workflow for failed or partially completed
translation runs. Translation writes recovery logs next to the output file, and
`repair` uses the local llama.cpp backend to retry from that recovery data:

```bash
./focst-local repair output_recovery.json
```
