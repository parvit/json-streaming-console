package main

import (
	"strings"
	"testing"
)

func TestGeminiFormatter_NewAndEnd(t *testing.T) {
	g := NewGeminiFormatter()
	if g == nil || g.totalUsage == nil {
		t.Fatalf("expected initialized GeminiFormatter")
	}

	// Test End() with no conversation ID and no usage
	out := captureOutput(true, func() {
		g.End()
	})
	if !strings.Contains(out, "| END            | - |") {
		t.Fatalf("expected clean END event with step_index '-', got: %q", out)
	}

	// Test End() idempotency (sync.Once)
	outSecond := captureOutput(true, func() {
		g.End()
	})
	if outSecond != "" {
		t.Fatalf("expected End() to be idempotent, but got: %q", outSecond)
	}
}

func TestGeminiFormatter_EndWithConversationIDAndUsage(t *testing.T) {
	g := NewGeminiFormatter()

	// Simulate format setting conversation_id and usage
	g.Format(map[string]any{
		"event":           "step_update",
		"conversation_id": "conv-test-123",
		"usage": map[string]any{
			"input_tokens":  float64(150),
			"output_tokens": int64(75),
			"total_tokens":  int(225),
		},
	})

	out := captureOutput(true, func() {
		g.End()
	})

	if !strings.Contains(out, "| END            | conv-test-123:- |") {
		t.Fatalf("expected END with conv-test-123:-, got: %q", out)
	}
	if !strings.Contains(out, "input_tokens:150") || !strings.Contains(out, "output_tokens:75") || !strings.Contains(out, "total_tokens:225") {
		t.Fatalf("expected accumulated token metrics in END recap, got: %q", out)
	}
}

func TestGeminiFormatter_AccumulateUsage(t *testing.T) {
	g := NewGeminiFormatter()

	// Invalid / non-map usage types should not cause panic or modifications
	g.accumulateUsage(nil)
	g.accumulateUsage("string_usage")
	g.accumulateUsage(123)

	// Valid map with float64, int64, and int
	g.accumulateUsage(map[string]any{
		"prompt":     float64(50),
		"completion": int64(25),
		"cached":     int(10),
	})

	if g.totalUsage["prompt"] != 50 || g.totalUsage["completion"] != 25 || g.totalUsage["cached"] != 10 {
		t.Fatalf("accumulated usage mismatch: %+v", g.totalUsage)
	}

	// Accumulate additional values to verify summation
	g.accumulateUsage(map[string]any{
		"prompt":     float64(50),
		"completion": int64(25),
	})

	if g.totalUsage["prompt"] != 100 || g.totalUsage["completion"] != 50 {
		t.Fatalf("accumulated usage summation mismatch: %+v", g.totalUsage)
	}
}

func TestGeminiFormatter_Format_InitDecomposition(t *testing.T) {
	g := NewGeminiFormatter()

	initEvent := map[string]any{
		"event":           "init",
		"conversation_id": "test-uuid-001",
		"init": map[string]any{
			"cwd":             "/app/workspace",
			"model":           "gemini-2.5-pro",
			"permission_mode": "DEFAULT",
			"metadata": map[string]any{
				"env": "prod",
			},
			"tools": []any{
				"bash",
				map[string]any{"name": "view_file"},
				map[string]any{"unknown": "data"},
				42,
			},
		},
	}

	out := captureOutput(true, func() {
		g.Format(initEvent)
	})

	if !strings.Contains(out, "| INIT           | test-uuid-001:- | cwd: /app/workspace") {
		t.Fatalf("expected cwd metadata line, got: %q", out)
	}
	if !strings.Contains(out, "| INIT           | test-uuid-001:- | model: gemini-2.5-pro") {
		t.Fatalf("expected model metadata line, got: %q", out)
	}
	if !strings.Contains(out, "| INIT           | test-uuid-001:- | allowed tool: bash") {
		t.Fatalf("expected allowed tool: bash, got: %q", out)
	}
	if !strings.Contains(out, "| INIT           | test-uuid-001:- | allowed tool: view_file") {
		t.Fatalf("expected allowed tool: view_file, got: %q", out)
	}
	if !strings.Contains(out, "| INIT           | test-uuid-001:- | allowed tool: map[unknown:data]") {
		t.Fatalf("expected allowed tool with map fallback, got: %q", out)
	}
	if !strings.Contains(out, "| INIT           | test-uuid-001:- | allowed tool: 42") {
		t.Fatalf("expected allowed tool with int fallback, got: %q", out)
	}
}

func TestGeminiFormatter_Format_InitToolVariants(t *testing.T) {
	// 1. Tools as []string
	g1 := NewGeminiFormatter()
	out1 := captureOutput(true, func() {
		g1.Format(map[string]any{
			"event": "init",
			"tools": []string{"toolA", "toolB"},
		})
	})
	if !strings.Contains(out1, "allowed tool: toolA") || !strings.Contains(out1, "allowed tool: toolB") {
		t.Fatalf("expected tools from []string, got: %q", out1)
	}

	// 2. Tools as JSON string
	g2 := NewGeminiFormatter()
	out2 := captureOutput(true, func() {
		g2.Format(map[string]any{
			"event": "init",
			"tools": "[\"tool_json1\", \"tool_json2\"]",
		})
	})
	if !strings.Contains(out2, "allowed tool: tool_json1") || !strings.Contains(out2, "allowed tool: tool_json2") {
		t.Fatalf("expected tools from JSON string, got: %q", out2)
	}

	// 3. Tools as single string
	g3 := NewGeminiFormatter()
	out3 := captureOutput(true, func() {
		g3.Format(map[string]any{
			"event": "init",
			"tools": "single_tool",
		})
	})
	if !strings.Contains(out3, "allowed tool: single_tool") {
		t.Fatalf("expected tools from single string, got: %q", out3)
	}

	// 4. Empty init event without other keys or tools
	g4 := NewGeminiFormatter()
	out4 := captureOutput(true, func() {
		g4.Format(map[string]any{
			"event": "init",
		})
	})
	if !strings.Contains(out4, "| INIT           | - |") {
		t.Fatalf("expected empty init event output, got: %q", out4)
	}
}

func TestGeminiFormatter_Format_StepUpdateAndTextDelta(t *testing.T) {
	g := NewGeminiFormatter()

	// Step update with agent_response and text_delta
	out := captureOutput(true, func() {
		g.Format(map[string]any{
			"event":           "step_update",
			"conversation_id": "c-999",
			"step_update": map[string]any{
				"step_index": float64(0),
				"step_type":  "agent_response",
				"text_delta": "Processing your request...",
			},
		})
	})

	if !strings.Contains(out, "WARN") {
		t.Fatalf("expected WARN level for step_update, got: %q", out)
	}
	if !strings.Contains(out, "| AGENT_RESPONSE | c-999:0 | Processing your request... |") {
		t.Fatalf("expected AGENT_RESPONSE step type and text_delta column, got: %q", out)
	}

	// Fallback to "type" when "event" is missing, and step_update without step_type
	out2 := captureOutput(true, func() {
		g.Format(map[string]any{
			"type":       "step_update",
			"step_index": "2",
		})
	})
	if !strings.Contains(out2, "| STEP_UPDATE    | -:2 |") {
		t.Fatalf("expected fallback STEP_UPDATE with step_index -:2, got: %q", out2)
	}
}

func TestGeminiFormatter_Format_ErrorState(t *testing.T) {
	g := NewGeminiFormatter()

	// Error state on step_update
	out := captureOutput(true, func() {
		g.Format(map[string]any{
			"event":           "step_update",
			"conversation_id": "c-err",
			"step_update": map[string]any{
				"step_index": float64(1),
				"state":      "ERROR",
				"message":    "Tool execution failed",
			},
		})
	})

	if !strings.Contains(out, "ERR") {
		t.Fatalf("expected ERR level for error state, got: %q", out)
	}

	// Status ERROR alternative
	out2 := captureOutput(true, func() {
		g.Format(map[string]any{
			"event":  "result",
			"status": "ERROR",
		})
	})
	if !strings.Contains(out2, "ERR") {
		t.Fatalf("expected ERR level for status: ERROR, got: %q", out2)
	}
}

func TestGeminiFormatter_Format_FieldTypeEncodings(t *testing.T) {
	g := NewGeminiFormatter()

	// Testing various field value types: string, int-float, float, bool, nil, map, unmarshallable channel
	ch := make(chan int)
	out := captureOutput(true, func() {
		g.Format(map[string]any{
			"event":        "result",
			"str_field":    "test_str",
			"int_field":    float64(42),
			"float_field":  12.345,
			"bool_field":   true,
			"nil_field":    nil,
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
	if !strings.Contains(out, "nil_field:NULL") {
		t.Fatalf("expected nil_field:NULL, got: %q", out)
	}
	if !strings.Contains(out, "obj_field:\"{\\\"K\\\":\\\"V\\\"}\"") {
		t.Fatalf("expected obj_field, got: %q", out)
	}
}

func TestGeminiFormatter_Format_MessageAndDataMerging(t *testing.T) {
	g := NewGeminiFormatter()

	out := captureOutput(true, func() {
		g.Format(map[string]any{
			"event": "step_update",
			"message": map[string]any{
				"msg_key": "msg_val",
			},
			"data": map[string]any{
				"data_key": "data_val",
			},
			"extra_key": "extra_val",
		})
	})

	if !strings.Contains(out, "msg_key:MSG_VAL") {
		t.Fatalf("expected merged msg_key:MSG_VAL, got: %q", out)
	}
	if !strings.Contains(out, "data_key:DATA_VAL") {
		t.Fatalf("expected merged data_key:DATA_VAL, got: %q", out)
	}
	if !strings.Contains(out, "extra_key:EXTRA_VAL") {
		t.Fatalf("expected merged extra_key:EXTRA_VAL, got: %q", out)
	}
}

func TestGeminiFormatter_Format_ANSIColors(t *testing.T) {
	g := NewGeminiFormatter()

	// Color for step_update (yellow)
	outYellow := captureOutput(false, func() {
		g.Format(map[string]any{
			"event": "step_update",
		})
	})
	if !strings.Contains(outYellow, colorYellow) {
		t.Fatalf("expected yellow ANSI escape in step_update, got: %q", outYellow)
	}

	// Color for error (red)
	outRed := captureOutput(false, func() {
		g.Format(map[string]any{
			"event": "step_update",
			"state": "ERROR",
		})
	})
	if !strings.Contains(outRed, colorRed) {
		t.Fatalf("expected red ANSI escape in error state, got: %q", outRed)
	}

	// Color for result (green)
	outGreen := captureOutput(false, func() {
		g.Format(map[string]any{
			"event": "result",
		})
	})
	if !strings.Contains(outGreen, colorGreen) {
		t.Fatalf("expected green ANSI escape in result, got: %q", outGreen)
	}
}
