package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
)

const (
	colorReset  = "\x1b[0m"
	colorRed    = "\x1b[31m"
	colorGreen  = "\x1b[32m"
	colorYellow = "\x1b[33m"
)

var (
	log         zerolog.Logger
	activeColor string
	version     = "dev"
)

type StreamFormatter interface {
	Format(map_data map[string]any)
	End()
}

func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixNano

	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
		PartsOrder: []string{
			zerolog.TimestampFieldName,
			zerolog.LevelFieldName,
			"event",
			"step_index",
			"text_delta",
			zerolog.MessageFieldName,
		},
		FieldsExclude: []string{"event", "step_index", "usage", "text_delta", "step_type", "conversation_id", "session_id"},
	}

	output.FormatPrepare = func(evt map[string]interface{}) error {
		activeColor = ""
		if output.NoColor {
			return nil
		}
		lvl, _ := evt[zerolog.LevelFieldName].(string)
		ev, _ := evt["event"].(string)
		state, _ := evt["state"].(string)

		if strings.EqualFold(state, "ERROR") || lvl == "error" {
			activeColor = colorRed
		} else if lvl == "warn" || strings.EqualFold(ev, "step_update") {
			activeColor = colorYellow
		} else if strings.EqualFold(ev, "init") || strings.EqualFold(ev, "result") || strings.EqualFold(ev, "end") {
			activeColor = colorGreen
		}
		return nil
	}

	output.FormatLevel = func(i interface{}) string {
		str := strings.ToUpper(fmt.Sprintf("%s", i))
		part := fmt.Sprintf("| %-5s", str)
		if activeColor != "" {
			return activeColor + part + colorReset
		}
		return part
	}
	output.FormatMessage = func(i interface{}) string {
		if i == nil {
			return ""
		}
		return fmt.Sprintf("%s", i)
	}
	output.FormatFieldName = func(i interface{}) string {
		return fmt.Sprintf("%s:", i)
	}
	output.FormatFieldValue = func(i interface{}) string {
		return strings.ToUpper(fmt.Sprintf("%s", i))
	}
	output.FormatPartValueByName = func(i interface{}, s string) string {
		switch s {
		case "event":
			val := "-"
			if i != nil && fmt.Sprintf("%s", i) != "" {
				val = strings.ToUpper(fmt.Sprintf("%s", i))
			}
			part := fmt.Sprintf("| %-14s", val)
			if activeColor != "" {
				return activeColor + part + colorReset
			}
			return part
		case "step_index":
			val := "-"
			if i != nil && fmt.Sprintf("%v", i) != "" {
				val = fmt.Sprintf("%v", i)
			}
			part := fmt.Sprintf("| %s |", val)
			if activeColor != "" {
				return activeColor + part + colorReset
			}
			return part
		case "text_delta":
			if i == nil {
				return ""
			}
			str := strings.TrimRight(fmt.Sprintf("%s", i), "\r\n")
			if str == "" {
				return ""
			}
			pipe := "|"
			if activeColor != "" {
				pipe = activeColor + "|" + colorReset
			}
			return fmt.Sprintf("%s %s", str, pipe)
		default:
			if i == nil {
				return ""
			}
			return fmt.Sprintf("%v", i)
		}
	}

	log = zerolog.New(output).With().Timestamp().Logger()
}

func main() {
	var versionFlag bool
	flag.BoolVar(&versionFlag, "version", false, "display application version")
	flag.BoolVar(&versionFlag, "v", false, "display application version (shorthand)")

	var formatFlag string
	flag.StringVar(&formatFlag, "format", "gemini", "streaming JSON format parser: gemini or claude")
	flag.StringVar(&formatFlag, "f", "gemini", "streaming JSON format parser (shorthand)")
	flag.Parse()

	if versionFlag {
		fmt.Printf("json-streaming-console %s\n", version)
		return
	}

	var formatter StreamFormatter
	switch strings.ToLower(strings.TrimSpace(formatFlag)) {
	case "gemini":
		formatter = NewGeminiFormatter()
	case "claude":
		formatter = NewClaudeFormatter()
	default:
		fmt.Fprintf(os.Stderr, "Unknown format: %q. Supported formats: gemini, claude\n", formatFlag)
		os.Exit(1)
	}

	defer func() {
		if err := recover(); err != nil {
			formatter.End()
			fmt.Printf("Fatal: %s\n", err)
			os.Exit(1)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		formatter.End()
		os.Exit(0)
	}()

	reader := bufio.NewReader(os.Stdin)
	for {
		textData, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(textData) > 0 {
					var map_data map[string]any
					if json.Unmarshal(textData, &map_data) == nil {
						formatter.Format(map_data)
					}
				}
				formatter.End()
				return
			}
			formatter.End()
			panic(err)
		}

		map_data := make(map[string]any)
		err = json.Unmarshal(textData, &map_data)
		if err == nil {
			formatter.Format(map_data)
		} else {
			log.Print(string(textData))
		}
	}
}
