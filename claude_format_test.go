package main

import (
	"strings"
	"testing"
)

func TestClaudeFormatter_NewAndEnd(t *testing.T) {
	c := NewClaudeFormatter()
	if c == nil || c.totalUsage == nil {
		t.Fatalf("expected initialized ClaudeFormatter")
	}

	// Test End() with no session ID and no usage
	out := captureOutput(true, func() {
		c.End()
	})
	if !strings.Contains(out, "| END            | - |") {
		t.Fatalf("expected clean END event with step_index '-', got: %q", out)
	}

	// Test End() idempotency (sync.Once)
	outSecond := captureOutput(true, func() {
		c.End()
	})
	if outSecond != "" {
		t.Fatalf("expected End() to be idempotent, but got: %q", outSecond)
	}
}

func TestClaudeFormatter_EndWithSessionIDAndUsage(t *testing.T) {
	c := NewClaudeFormatter()

	// Simulate format setting session_id and usage
	c.Format(map[string]any{
		"type":       "assistant",
		"session_id": "claude-sess-456",
		"usage": map[string]any{
			"input_tokens":  float64(300),
			"output_tokens": int64(150),
		},
	})

	out := captureOutput(true, func() {
		c.End()
	})

	if !strings.Contains(out, "| END            | claude-sess-456:- |") {
		t.Fatalf("expected END with claude-sess-456:-, got: %q", out)
	}
	// total_tokens should be automatically synthesized: 300 + 150 = 450
	if !strings.Contains(out, "input_tokens:300") || !strings.Contains(out, "output_tokens:150") || !strings.Contains(out, "total_tokens:450") {
		t.Fatalf("expected synthesized total_tokens and usage counters, got: %q", out)
	}
}

func TestClaudeFormatter_AccumulateUsage(t *testing.T) {
	c := NewClaudeFormatter()

	// Non-map types should not cause panic
	c.accumulateUsage(nil)
	c.accumulateUsage("string_val")

	// Map with input_tokens, output_tokens, int type, and pre-existing total_tokens
	c.accumulateUsage(map[string]any{
		"input_tokens":  float64(100),
		"output_tokens": int64(50),
		"total_tokens":  int(150),
		"cache_read":    int(20),
	})

	if c.totalUsage["input_tokens"] != 100 || c.totalUsage["output_tokens"] != 50 || c.totalUsage["total_tokens"] != 150 || c.totalUsage["cache_read"] != 20 {
		t.Fatalf("usage mismatch: %+v", c.totalUsage)
	}

	// Message level usage extraction in Format
	c.Format(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"usage": map[string]any{
				"input_tokens": float64(50),
			},
		},
	})

	if c.totalUsage["input_tokens"] != 150 {
		t.Fatalf("expected input_tokens to accumulate from message, got: %d", c.totalUsage["input_tokens"])
	}
}

func TestClaudeFormatter_Format_InitDecomposition(t *testing.T) {
	c := NewClaudeFormatter()

	initEvent := map[string]any{
		"type":       "system",
		"subtype":    "init",
		"session_id": "sess-init-01",
		"model":      "claude-3-7-sonnet",
		"cwd":        "/home/project",
		"config": map[string]any{
			"auto_approve": true,
		},
		"tools": []any{
			"read_file",
			map[string]any{"name": "execute_bash"},
			map[string]any{"custom": "def"},
			99,
		},
	}

	out := captureOutput(true, func() {
		c.Format(initEvent)
	})

	if !strings.Contains(out, "| INIT           | sess-init-01:- | cwd: /home/project") {
		t.Fatalf("expected cwd metadata line, got: %q", out)
	}
	if !strings.Contains(out, "| INIT           | sess-init-01:- | model: claude-3-7-sonnet") {
		t.Fatalf("expected model metadata line, got: %q", out)
	}
	if !strings.Contains(out, "| INIT           | sess-init-01:- | allowed tool: read_file") {
		t.Fatalf("expected allowed tool: read_file, got: %q", out)
	}
	if !strings.Contains(out, "| INIT           | sess-init-01:- | allowed tool: execute_bash") {
		t.Fatalf("expected allowed tool: execute_bash, got: %q", out)
	}
	if !strings.Contains(out, "| INIT           | sess-init-01:- | allowed tool: map[custom:def]") {
		t.Fatalf("expected allowed tool map fallback, got: %q", out)
	}
	if !strings.Contains(out, "| INIT           | sess-init-01:- | allowed tool: 99") {
		t.Fatalf("expected allowed tool int fallback, got: %q", out)
	}

	// Tool variants: []string and JSON string
	c2 := NewClaudeFormatter()
	out2 := captureOutput(true, func() {
		c2.Format(map[string]any{
			"type":  "init",
			"tools": []string{"edit_file"},
		})
	})
	if !strings.Contains(out2, "allowed tool: edit_file") {
		t.Fatalf("expected tool from []string, got: %q", out2)
	}

	c3 := NewClaudeFormatter()
	out3 := captureOutput(true, func() {
		c3.Format(map[string]any{
			"subtype": "init",
			"tools":   "[\"grep_search\"]",
		})
	})
	if !strings.Contains(out3, "allowed tool: grep_search") {
		t.Fatalf("expected tool from JSON string, got: %q", out3)
	}

	// Empty init
	c4 := NewClaudeFormatter()
	out4 := captureOutput(true, func() {
		c4.Format(map[string]any{
			"type": "init",
		})
	})
	if !strings.Contains(out4, "| INIT           | - |") {
		t.Fatalf("expected empty init, got: %q", out4)
	}
}

func TestClaudeFormatter_Format_Result(t *testing.T) {
	c := NewClaudeFormatter()

	// 1. Success result with duration_ms, cost_usd, num_turns
	outSuccess := captureOutput(true, func() {
		c.Format(map[string]any{
			"type":            "result",
			"conversation_id": "cid-res",
			"duration_ms":     float64(2500),
			"cost_usd":        0.0125,
			"num_turns":       float64(3),
			"extra_info":      "complete",
		})
	})

	if !strings.Contains(outSuccess, "INF") {
		t.Fatalf("expected INF level for successful result, got: %q", outSuccess)
	}
	if !strings.Contains(outSuccess, "| RESULT         | cid-res:- |") {
		t.Fatalf("expected RESULT event with cid-res:-, got: %q", outSuccess)
	}
	if !strings.Contains(outSuccess, "status:SUCCESS") {
		t.Fatalf("expected status:SUCCESS, got: %q", outSuccess)
	}
	if !strings.Contains(outSuccess, "duration_seconds:2.5") {
		t.Fatalf("expected duration_seconds:2.5, got: %q", outSuccess)
	}
	if !strings.Contains(outSuccess, "cost_usd:0.0125") {
		t.Fatalf("expected cost_usd:0.0125, got: %q", outSuccess)
	}
	if !strings.Contains(outSuccess, "num_turns:3") {
		t.Fatalf("expected num_turns:3, got: %q", outSuccess)
	}

	// 2. Error result with duration_seconds and subtype error
	outError := captureOutput(true, func() {
		c.Format(map[string]any{
			"type":             "result",
			"subtype":          "error",
			"duration_seconds": 1.75,
		})
	})
	if !strings.Contains(outError, "ERR") {
		t.Fatalf("expected ERR level for error result, got: %q", outError)
	}
	if !strings.Contains(outError, "status:ERROR") {
		t.Fatalf("expected status:ERROR, got: %q", outError)
	}
	if !strings.Contains(outError, "duration_seconds:1.75") {
		t.Fatalf("expected duration_seconds:1.75, got: %q", outError)
	}

	// 3. Result with is_error = true and custom subtype
	outCustom := captureOutput(true, func() {
		c.Format(map[string]any{
			"type":     "result",
			"is_error": true,
			"subtype":  "timeout",
		})
	})
	if !strings.Contains(outCustom, "ERR") {
		t.Fatalf("expected ERR level for is_error result, got: %q", outCustom)
	}
	if !strings.Contains(outCustom, "status:TIMEOUT") {
		t.Fatalf("expected status:TIMEOUT, got: %q", outCustom)
	}
}

func TestClaudeFormatter_Format_Error(t *testing.T) {
	c := NewClaudeFormatter()

	// Error event with error object
	outObj := captureOutput(true, func() {
		c.Format(map[string]any{
			"type": "error",
			"error": map[string]any{
				"message": "Rate limit exceeded",
			},
			"code": 429,
		})
	})
	if !strings.Contains(outObj, "ERR") {
		t.Fatalf("expected ERR level for error event, got: %q", outObj)
	}
	if !strings.Contains(outObj, "| ERROR          | - |") {
		t.Fatalf("expected ERROR event, got: %q", outObj)
	}
	if !strings.Contains(outObj, "Rate limit exceeded") {
		t.Fatalf("expected error message, got: %q", outObj)
	}

	// Error event with string message
	outMsg := captureOutput(true, func() {
		c.Format(map[string]any{
			"type":    "error",
			"message": "Connection reset by peer",
		})
	})
	if !strings.Contains(outMsg, "Connection reset by peer") {
		t.Fatalf("expected string error message, got: %q", outMsg)
	}
}

func TestClaudeFormatter_Format_StepsAndDeltas(t *testing.T) {
	c := NewClaudeFormatter()

	// 1. Assistant event with text_delta and step_index
	out1 := captureOutput(true, func() {
		c.Format(map[string]any{
			"type":       "assistant",
			"session_id": "sess-step-1",
			"step_index": float64(0),
			"text_delta": "Thinking about plan...",
		})
	})
	if !strings.Contains(out1, "| AGENT_RESPONSE | sess-step-1:0 | Thinking about plan... |") {
		t.Fatalf("expected AGENT_RESPONSE with text delta, got: %q", out1)
	}

	// 2. User event with delta.text and index
	out2 := captureOutput(true, func() {
		c.Format(map[string]any{
			"type":  "user",
			"index": float64(1),
			"delta": map[string]any{
				"text": "User question here",
			},
		})
	})
	if !strings.Contains(out2, "| USER_INPUT     | sess-step-1:1 | User question here |") {
		t.Fatalf("expected USER_INPUT with inherited session_id, got: %q", out2)
	}

	// 3. content_block_delta with delta.text_delta
	out3 := captureOutput(true, func() {
		c.Format(map[string]any{
			"type": "content_block_delta",
			"delta": map[string]any{
				"text_delta": "Chunk stream text",
			},
		})
	})
	if !strings.Contains(out3, "| AGENT_RESPONSE | sess-step-1:1 | Chunk stream text |") {
		t.Fatalf("expected AGENT_RESPONSE with incremented step counter, got: %q", out3)
	}

	// 4. Message content containing tool_use
	out4 := captureOutput(true, func() {
		c.Format(map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"content": []any{
					map[string]any{
						"type": "tool_use",
						"name": "bash_exec",
					},
				},
			},
		})
	})
	if !strings.Contains(out4, "| TOOL_USE       |") {
		t.Fatalf("expected TOOL_USE from message content, got: %q", out4)
	}
	if !strings.Contains(out4, "tool_name:BASH_EXEC") {
		t.Fatalf("expected tool_name:BASH_EXEC, got: %q", out4)
	}

	// 5. Message content containing text block
	out5 := captureOutput(true, func() {
		c.Format(map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"content": []any{
					map[string]any{
						"type": "text",
						"text": "Message content text",
					},
				},
			},
		})
	})
	if !strings.Contains(out5, "Message content text |") {
		t.Fatalf("expected text from message content, got: %q", out5)
	}

	// 6. Direct tool_use and tool_result events
	outToolUse := captureOutput(true, func() {
		c.Format(map[string]any{"type": "tool_use"})
	})
	if !strings.Contains(outToolUse, "| TOOL_USE       |") {
		t.Fatalf("expected TOOL_USE event, got: %q", outToolUse)
	}

	outToolRes := captureOutput(true, func() {
		c.Format(map[string]any{"type": "tool_result"})
	})
	if !strings.Contains(outToolRes, "| TOOL_RESULT    |") {
		t.Fatalf("expected TOOL_RESULT event, got: %q", outToolRes)
	}

	// 7. Custom and empty event type
	outCustom := captureOutput(true, func() {
		c.Format(map[string]any{"type": "custom_evt"})
	})
	if !strings.Contains(outCustom, "| CUSTOM_EVT     |") {
		t.Fatalf("expected CUSTOM_EVT, got: %q", outCustom)
	}

	outEmpty := captureOutput(true, func() {
		c.Format(map[string]any{})
	})
	if !strings.Contains(outEmpty, "| STEP_UPDATE    |") {
		t.Fatalf("expected STEP_UPDATE default, got: %q", outEmpty)
	}
}

func TestClaudeFormatter_Format_FieldTypeEncodings(t *testing.T) {
	c := NewClaudeFormatter()

	ch := make(chan int)
	out := captureOutput(true, func() {
		c.Format(map[string]any{
			"type":         "custom",
			"str_field":    "test_str",
			"int_field":    float64(42),
			"float_field":  12.345,
			"bool_field":   true,
			"obj_field":    map[string]any{"k": "v"},
			"unmarshal_ch": ch,
		})
	})

	if !strings.Contains(out, "str_field:TEST_STR") {
		t.Fatalf("expected str_field:TEST_STR, got: %q", out)
	}
	if !strings.Contains(out, "int_field:42") {
		t.Fatalf("expected int_field:42, got: %q", out)
	}
	if !strings.Contains(out, "float_field:12.345") {
		t.Fatalf("expected float_field:12.345, got: %q", out)
	}
	if !strings.Contains(out, "bool_field:TRUE") {
		t.Fatalf("expected bool_field:TRUE, got: %q", out)
	}
	if !strings.Contains(out, "obj_field:\"{\\\"K\\\":\\\"V\\\"}\"") {
		t.Fatalf("expected obj_field, got: %q", out)
	}
}
