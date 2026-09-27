# json-streaming-console

A very basic terminal streaming parser and structured formatter for Newline-Delimited JSON (NDJSON) output emitted by CLI LLM tools, like Google Antigravity CLI (`agy`) with `--output-format stream-json`.

Built using in Go library [`zerolog`](https://github.com/rs/zerolog), `json-streaming-console` converts raw streaming JSON envelopes into a readable, ANSI-colorized tabular console output.

Allows to debug the behavior of the cli tools which can exhibit some `interesting` behavior when requesting unexpected file access or tool calls.

## Output Structure

```
2026-09-27T12:43:48+02:00 | INFO  | INIT           | 4D29D340...:- | cwd: C:\home\dev
2026-09-27T12:43:48+02:00 | INFO  | INIT           | 4D29D340...:- | model: gemini-2.5-pro
2026-09-27T12:43:48+02:00 | INFO  | INIT           | 4D29D340...:- | allowed tool: view_file
2026-09-27T12:43:48+02:00 | INFO  | INIT           | 4D29D340...:- | allowed tool: run_command
2026-09-27T12:43:48+02:00 | WARN  | USER_INPUT     | 4D29D340...:0 | STATE:DONE
2026-09-27T12:43:49+02:00 | WARN  | AGENT_RESPONSE | 4D29D340...:1 | I will inspect the code. | STATE:ACTIVE
2026-09-27T12:43:50+02:00 | WARN  | TOOL           | 4D29D340...:2 | STATE:ACTIVE TOOL_NAME:VIEW_FILE
2026-09-27T12:43:51+02:00 | INFO  | RESULT         | 4D29D340...:- | DURATION_SECONDS:3.2 STATUS:SUCCESS
2026-09-27T12:43:52+02:00 | INFO  | END            | 4D29D340...:- | INPUT_TOKENS:1500 OUTPUT_TOKENS:200 TOTAL_TOKENS:1700
```

## Usage

Pipe NDJSON streaming output from `agy` or other LLM CLI tools via `stdin`:

```sh
agy --output-format stream-json "your prompt here" | json-streaming-console
```

Or replay an NDJSON log file:

```sh
cat events.ndjson | json-streaming-console
```

## Requirements

- Go 1.25+
- Dependencies: `github.com/rs/zerolog`
