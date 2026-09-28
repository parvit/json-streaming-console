# json-streaming-console

A real-time terminal streaming parser and structured formatter for Newline-Delimited JSON (NDJSON) output emitted by CLI LLM tools, such as Google Antigravity CLI (`agy`) and Anthropic Claude Code (`claude`).

Built in Go using [`zerolog`](https://github.com/rs/zerolog), `json-streaming-console` converts verbose raw streaming JSON envelopes into a readable, ANSI-colorized tabular console output. It is particularly useful for observing and debugging CLI agent behavior (tool calls, file access, token consumption, and intermediate steps) in real time.

## Supported Formats

| Format Flag (`-format`, `-f`) | Target Tool | Supported Stream Events |
| :--- | :--- | :--- |
| `gemini` *(default)* | Google Antigravity CLI (`agy`) | `init`, `step_update` (`user_input`, `agent_response`, `tool`), `result`, `end` |
| `claude` | Anthropic Claude Code CLI (`claude`) | `system`/`init`, `assistant`, `content_block_delta`, `tool_use`, `tool_result`, `result`, `error` |

## Column Layout

The output is formatted as a continuous single-pipe table:

```
[TIMESTAMP] | [LEVEL] | [EVENT] | [SESSION_ID:STEP] | [TEXT_DELTA |] [ATTRIBUTES...]
```

- **`TIMESTAMP`**: Event timestamp in RFC3339 format.
- **`LEVEL`**: Categorized severity level:
  - `INFO` (Green): Lifecycle milestones (`INIT`, `RESULT`, `END`).
  - `WARN` (Yellow): Active execution turns, step updates, tool invocations, and agent responses.
  - `ERROR` (Red): Execution failures and events reporting error states.
- **`EVENT`**: Granular event or step classifier (e.g. `INIT`, `USER_INPUT`, `AGENT_RESPONSE`, `TOOL_USE`, `TOOL_RESULT`, `RESULT`, `END`).
- **`SESSION_ID:STEP`**: Unique session/conversation ID combined with the turn step index (`<id>:<step>`). Defaults to `<id>:-` when step index is absent.
- **`TEXT_DELTA`**: Dedicated field displaying real-time streaming chunks of agent response text, separated cleanly by a trailing pipe (`|`). Omitted on non-text steps.
- **`ATTRIBUTES`**: Contextual key-value metadata (e.g. `STATE:DONE`, `TOOL_NAME:VIEW_FILE`, `DURATION_SECONDS:3.2`, `STATUS:SUCCESS`). Excludes redundant internal fields.

## Output Examples

### Output Example

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

This form defaults to Google Gemini's cli format.

```sh
agy --output-format stream-json "your prompt here" | json-streaming-console
```

By specifying the format explicitly it can be used for Claude Code also.

```sh
claude -p --output-format stream-json --verbose "your prompt here" | json-streaming-console -format claude
```

## CLI Flags

| Flag | Shorthand | Type | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `-format` | `-f` | `string` | `gemini` | Streaming JSON parser format: `gemini` or `claude` |

## Requirements

- Go 1.25+
- Dependencies: `github.com/rs/zerolog`
