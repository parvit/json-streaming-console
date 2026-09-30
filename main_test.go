package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestNewConsoleWriter_FormatPrepare(t *testing.T) {
	var buf bytes.Buffer

	tests := []struct {
		name          string
		noColor       bool
		evt           map[string]interface{}
		expectedColor string
	}{
		{
			name:    "NoColor true returns empty color",
			noColor: true,
			evt: map[string]interface{}{
				"state": "ERROR",
			},
			expectedColor: "",
		},
		{
			name:    "State ERROR sets red",
			noColor: false,
			evt: map[string]interface{}{
				"state": "ERROR",
			},
			expectedColor: colorRed,
		},
		{
			name:    "Level error sets red",
			noColor: false,
			evt: map[string]interface{}{
				zerolog.LevelFieldName: "error",
			},
			expectedColor: colorRed,
		},
		{
			name:    "Level warn sets yellow",
			noColor: false,
			evt: map[string]interface{}{
				zerolog.LevelFieldName: "warn",
			},
			expectedColor: colorYellow,
		},
		{
			name:    "Event step_update sets yellow",
			noColor: false,
			evt: map[string]interface{}{
				"event": "step_update",
			},
			expectedColor: colorYellow,
		},
		{
			name:    "Event init sets green",
			noColor: false,
			evt: map[string]interface{}{
				"event": "init",
			},
			expectedColor: colorGreen,
		},
		{
			name:    "Event result sets green",
			noColor: false,
			evt: map[string]interface{}{
				"event": "result",
			},
			expectedColor: colorGreen,
		},
		{
			name:    "Event end sets green",
			noColor: false,
			evt: map[string]interface{}{
				"event": "end",
			},
			expectedColor: colorGreen,
		},
		{
			name:    "Other events leave color empty",
			noColor: false,
			evt: map[string]interface{}{
				"event":                "other",
				zerolog.LevelFieldName: "info",
			},
			expectedColor: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testWriter := newConsoleWriter(&buf, tc.noColor)
			_ = testWriter.FormatPrepare(tc.evt)
			if activeColor != tc.expectedColor {
				t.Fatalf("expected color %q, got %q", tc.expectedColor, activeColor)
			}
		})
	}
}

func TestNewConsoleWriter_FormatLevel(t *testing.T) {
	var buf bytes.Buffer
	cw := newConsoleWriter(&buf, false)

	activeColor = ""
	out := cw.FormatLevel("info")
	expected := "| INFO "
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}

	activeColor = colorGreen
	out = cw.FormatLevel("info")
	expectedColored := colorGreen + "| INFO " + colorReset
	if out != expectedColored {
		t.Fatalf("expected %q, got %q", expectedColored, out)
	}
	activeColor = ""
}

func TestNewConsoleWriter_FormatMessage(t *testing.T) {
	var buf bytes.Buffer
	cw := newConsoleWriter(&buf, false)

	if cw.FormatMessage(nil) != "" {
		t.Fatalf("expected empty string for nil message")
	}

	msg := cw.FormatMessage("hello test")
	if msg != "hello test" {
		t.Fatalf("expected 'hello test', got %q", msg)
	}
}

func TestNewConsoleWriter_FormatFieldNameAndValue(t *testing.T) {
	var buf bytes.Buffer
	cw := newConsoleWriter(&buf, false)

	fn := cw.FormatFieldName("status")
	if fn != "status:" {
		t.Fatalf("expected 'status:', got %q", fn)
	}

	fv := cw.FormatFieldValue("success")
	if fv != "SUCCESS" {
		t.Fatalf("expected 'SUCCESS', got %q", fv)
	}
}

func TestNewConsoleWriter_FormatPartValueByName(t *testing.T) {
	var buf bytes.Buffer
	cw := newConsoleWriter(&buf, false)

	// Case "event"
	activeColor = ""
	if out := cw.FormatPartValueByName(nil, "event"); out != "| -             " {
		t.Fatalf("unexpected output for nil event: %q", out)
	}
	if out := cw.FormatPartValueByName("", "event"); out != "| -             " {
		t.Fatalf("unexpected output for empty event: %q", out)
	}
	if out := cw.FormatPartValueByName("init", "event"); out != "| INIT          " {
		t.Fatalf("unexpected output for init event: %q", out)
	}
	activeColor = colorGreen
	if out := cw.FormatPartValueByName("init", "event"); out != colorGreen+"| INIT          "+colorReset {
		t.Fatalf("unexpected output for colored init event: %q", out)
	}
	activeColor = ""

	// Case "step_index"
	if out := cw.FormatPartValueByName(nil, "step_index"); out != "| - |" {
		t.Fatalf("unexpected output for nil step_index: %q", out)
	}
	if out := cw.FormatPartValueByName("", "step_index"); out != "| - |" {
		t.Fatalf("unexpected output for empty step_index: %q", out)
	}
	if out := cw.FormatPartValueByName("session:1", "step_index"); out != "| session:1 |" {
		t.Fatalf("unexpected output for session:1 step_index: %q", out)
	}
	activeColor = colorYellow
	if out := cw.FormatPartValueByName("session:1", "step_index"); out != colorYellow+"| session:1 |"+colorReset {
		t.Fatalf("unexpected output for colored step_index: %q", out)
	}
	activeColor = ""

	// Case "text_delta"
	if out := cw.FormatPartValueByName(nil, "text_delta"); out != "" {
		t.Fatalf("expected empty for nil text_delta, got %q", out)
	}
	if out := cw.FormatPartValueByName("\r\n", "text_delta"); out != "" {
		t.Fatalf("expected empty for newline-only text_delta, got %q", out)
	}
	if out := cw.FormatPartValueByName("chunk", "text_delta"); out != "chunk |" {
		t.Fatalf("expected 'chunk |', got %q", out)
	}
	activeColor = colorYellow
	if out := cw.FormatPartValueByName("chunk", "text_delta"); out != "chunk "+colorYellow+"|"+colorReset {
		t.Fatalf("expected colored pipe for text_delta, got %q", out)
	}
	activeColor = ""

	// Default case
	if out := cw.FormatPartValueByName(nil, "custom"); out != "" {
		t.Fatalf("expected empty for nil custom field, got %q", out)
	}
	if out := cw.FormatPartValueByName(12345, "custom"); out != "12345" {
		t.Fatalf("expected '12345' for custom field, got %q", out)
	}
}

func TestGetFormatter(t *testing.T) {
	fmtGemini, err := getFormatter("gemini")
	if err != nil || fmtGemini == nil {
		t.Fatalf("expected gemini formatter, got err: %v", err)
	}
	if _, ok := fmtGemini.(*GeminiFormatter); !ok {
		t.Fatalf("expected *GeminiFormatter type")
	}

	fmtClaude, err := getFormatter(" claude ")
	if err != nil || fmtClaude == nil {
		t.Fatalf("expected claude formatter, got err: %v", err)
	}
	if _, ok := fmtClaude.(*ClaudeFormatter); !ok {
		t.Fatalf("expected *ClaudeFormatter type")
	}

	fmtUnknown, err := getFormatter("invalid_format")
	if err == nil || fmtUnknown != nil {
		t.Fatalf("expected error for invalid_format")
	}
	if !strings.Contains(err.Error(), "Unknown format") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestProcessStream_ValidJSON(t *testing.T) {
	ndjson := "{\"event\":\"init\",\"conversation_id\":\"c1\"}\n{\"event\":\"result\",\"conversation_id\":\"c1\"}\n"
	mock := &mockFormatter{}

	err := processStream(strings.NewReader(ndjson), mock)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(mock.formatted) != 2 {
		t.Fatalf("expected 2 formatted events, got: %d", len(mock.formatted))
	}
	if mock.endCalled != 1 {
		t.Fatalf("expected End() to be called once, got: %d", mock.endCalled)
	}
}

func TestProcessStream_WithoutTrailingNewline(t *testing.T) {
	ndjson := "{\"event\":\"result\",\"conversation_id\":\"c1\"}"
	mock := &mockFormatter{}

	err := processStream(strings.NewReader(ndjson), mock)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(mock.formatted) != 1 {
		t.Fatalf("expected 1 formatted event, got: %d", len(mock.formatted))
	}
	if mock.endCalled != 1 {
		t.Fatalf("expected End() to be called once, got: %d", mock.endCalled)
	}
}

func TestProcessStream_InvalidJSON(t *testing.T) {
	ndjson := "Not a JSON line\n{\"event\":\"result\"}\n"
	mock := &mockFormatter{}

	out := captureOutput(true, func() {
		err := processStream(strings.NewReader(ndjson), mock)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	if len(mock.formatted) != 1 {
		t.Fatalf("expected 1 formatted event, got: %d", len(mock.formatted))
	}
	if !strings.Contains(out, "Not a JSON line") {
		t.Fatalf("expected raw line to be logged, got: %q", out)
	}
	if mock.endCalled != 1 {
		t.Fatalf("expected End() to be called once, got: %d", mock.endCalled)
	}
}

func TestProcessStream_ReaderError(t *testing.T) {
	readErr := errors.New("simulated pipe read error")
	r := &errorReader{err: readErr}
	mock := &mockFormatter{}

	err := processStream(r, mock)
	if !errors.Is(err, readErr) {
		t.Fatalf("expected read error, got: %v", err)
	}
	if mock.endCalled != 1 {
		t.Fatalf("expected End() to be called on error, got: %d", mock.endCalled)
	}
}

func TestRun_Flags(t *testing.T) {
	// Test -version flag
	var stdout, stderr bytes.Buffer
	code := run([]string{"-version"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for -version, got %d", code)
	}
	if !strings.Contains(stdout.String(), "json-streaming-console") {
		t.Fatalf("expected version output, got: %q", stdout.String())
	}

	// Test -v flag
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"-v"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for -v, got %d", code)
	}
	if !strings.Contains(stdout.String(), "json-streaming-console") {
		t.Fatalf("expected version output, got: %q", stdout.String())
	}

	// Test invalid format
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"-format", "unsupported"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1 for unsupported format, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Unknown format") {
		t.Fatalf("expected unknown format error in stderr, got: %q", stderr.String())
	}

	// Test invalid flag
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"-unknown-flag"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1 for invalid flag, got %d", code)
	}

	// Test successful stream run with claude
	stdout.Reset()
	stderr.Reset()
	input := "{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"s1\"}\n"
	code = run([]string{"-f", "claude"}, strings.NewReader(input), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr: %s", code, stderr.String())
	}

	// Test processStream error in run
	stdout.Reset()
	stderr.Reset()
	r := &errorReader{err: errors.New("fatal input failure")}
	code = run([]string{"-format", "gemini"}, r, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1 on stream error, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Fatal: fatal input failure") {
		t.Fatalf("expected Fatal message in stderr, got: %q", stderr.String())
	}
}
