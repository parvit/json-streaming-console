package main

import (
	"bytes"
	"sync"

	"github.com/rs/zerolog"
)

var testLogMu sync.Mutex

func captureOutput(noColor bool, fn func()) string {
	testLogMu.Lock()
	defer testLogMu.Unlock()

	var buf bytes.Buffer
	cw := newConsoleWriter(&buf, noColor)
	oldLog := log
	log = zerolog.New(cw).With().Timestamp().Logger()
	defer func() {
		log = oldLog
		activeColor = ""
	}()

	fn()
	return buf.String()
}

type mockFormatter struct {
	formatted []map[string]any
	endCalled int
}

func (m *mockFormatter) Format(d map[string]any) {
	m.formatted = append(m.formatted, d)
}

func (m *mockFormatter) End() {
	m.endCalled++
}

type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, e.err
}
