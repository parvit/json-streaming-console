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
	"sort"
	"strings"
	"sync"
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
	log                zerolog.Logger
	suppressProgress   *bool
	totalUsage         = make(map[string]int64)
	usageMu            sync.Mutex
	activeColor        string
	endOnce            sync.Once
	lastConversationID string
)

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
		FieldsExclude: []string{"event", "step_index", "usage", "text_delta", "step_type", "conversation_id"},
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

	log = zerolog.New(output).With().
		Timestamp().
		Logger()
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("Fatal: %s\n", err)
			os.Exit(1)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		os.Exit(0)
	}()

	suppressProgress = flag.Bool("suppress-progress", false, "Controls whether progress messages are printed or not")
	flag.Parse()
	if !flag.Parsed() {
		flag.PrintDefaults()
		os.Exit(1)
	}

	defer logEndMessage()

	reader := bufio.NewReader(os.Stdin)
	for {
		textData, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(textData) > 0 {
					var map_data map[string]any
					if json.Unmarshal(textData, &map_data) == nil {
						logGeminiFormat(map_data)
					}
				}
				return
			}
			panic(err)
		}

		map_data := make(map[string]any)
		err = json.Unmarshal(textData, &map_data)
		if err == nil {
			logGeminiFormat(map_data)
		} else {
			log.Print(string(textData))
		}
	}
}

func logEndMessage() {
	endOnce.Do(func() {
		usageMu.Lock()
		keys := make([]string, 0, len(totalUsage))
		for k := range totalUsage {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		cID := lastConversationID
		endLog := log.Info().Str("event", "end")
		if cID != "" {
			endLog = endLog.Str("step_index", fmt.Sprintf("%s:-", cID))
		} else {
			endLog = endLog.Str("step_index", "-")
		}
		for _, k := range keys {
			endLog = endLog.Int64(k, totalUsage[k])
		}
		usageMu.Unlock()
		endLog.Send()
	})
}

func logGeminiFormat(map_data map[string]any) {
	eventName, _ := map_data["event"].(string)
	if eventName == "" {
		if t, ok := map_data["type"].(string); ok {
			eventName = t
		}
	}

	accumulateUsage := func(u any) {
		if uMap, ok := u.(map[string]any); ok {
			usageMu.Lock()
			defer usageMu.Unlock()
			for k, v := range uMap {
				switch val := v.(type) {
				case float64:
					totalUsage[k] += int64(val)
				case int64:
					totalUsage[k] += val
				case int:
					totalUsage[k] += int64(val)
				}
			}
		}
	}

	if u, ok := map_data["usage"]; ok {
		accumulateUsage(u)
		delete(map_data, "usage")
	}

	fields := make(map[string]any)
	if eventName != "" {
		fields["event"] = eventName
		if payload, ok := map_data[eventName].(map[string]any); ok {
			if u, ok := payload["usage"]; ok {
				accumulateUsage(u)
				delete(payload, "usage")
			}
			for k, v := range payload {
				if k != "usage" {
					fields[k] = v
				}
			}
		}
	}
	if msg, ok := map_data["message"].(map[string]any); ok {
		for k, v := range msg {
			if _, exists := fields[k]; !exists {
				fields[k] = v
			}
		}
	}
	if data, ok := map_data["data"].(map[string]any); ok {
		for k, v := range data {
			if _, exists := fields[k]; !exists {
				fields[k] = v
			}
		}
	}
	for k, v := range map_data {
		if k != "event" && k != eventName && k != "message" && k != "data" && k != "usage" {
			if _, exists := fields[k]; !exists {
				fields[k] = v
			}
		}
	}

	convID := "-"
	if c, ok := fields["conversation_id"].(string); ok && c != "" {
		convID = c
		usageMu.Lock()
		lastConversationID = c
		usageMu.Unlock()
	}

	if strings.EqualFold(eventName, "init") {
		logInitFormat(fields, convID)
		return
	}

	eventField := eventName
	stepType, _ := fields["step_type"].(string)
	if strings.EqualFold(eventName, "step_update") && stepType != "" {
		eventField = stepType
	}

	stateStr, _ := fields["state"].(string)
	if stateStr == "" {
		stateStr, _ = fields["status"].(string)
	}
	isError := strings.EqualFold(stateStr, "ERROR")

	var eventLog *zerolog.Event
	if isError {
		eventLog = log.Error()
	} else if strings.EqualFold(eventName, "step_update") {
		eventLog = log.Warn()
		if *suppressProgress {
			eventLog.Discard()
		}
	} else if strings.EqualFold(eventName, "result") || strings.EqualFold(eventName, "end") {
		eventLog = log.Info()
	} else {
		eventLog = log.Info()
	}

	if eventField != "" {
		eventLog = eventLog.Str("event", eventField)
	}

	stepIdx := "-"
	if si, ok := fields["step_index"]; ok {
		if f, ok := si.(float64); ok {
			stepIdx = fmt.Sprintf("%d", int64(f))
		} else if s := fmt.Sprintf("%v", si); s != "" {
			stepIdx = s
		}
	}

	stepIndexVal := "-"
	if convID != "-" || stepIdx != "-" {
		stepIndexVal = fmt.Sprintf("%s:%s", convID, stepIdx)
	}
	eventLog = eventLog.Str("step_index", stepIndexVal)

	if td, ok := fields["text_delta"].(string); ok && td != "" {
		eventLog = eventLog.Str("text_delta", td)
	}

	for k, v := range fields {
		if k == "event" || k == "step_index" || k == "usage" || k == "text_delta" || k == "step_type" || k == "conversation_id" {
			continue
		}
		switch val := v.(type) {
		case string:
			eventLog = eventLog.Str(k, val)
		case float64:
			if val == float64(int64(val)) {
				eventLog = eventLog.Int64(k, int64(val))
			} else {
				eventLog = eventLog.Float64(k, val)
			}
		case bool:
			eventLog = eventLog.Bool(k, val)
		case nil:
			eventLog = eventLog.Str(k, "null")
		default:
			if b, err := json.Marshal(val); err == nil {
				eventLog = eventLog.Str(k, string(b))
			} else {
				eventLog = eventLog.Str(k, fmt.Sprintf("%v", val))
			}
		}
	}

	eventLog.Send()
}

func logInitFormat(fields map[string]any, convID string) {
	stepIndexVal := fmt.Sprintf("%s:-", convID)
	if convID == "-" {
		stepIndexVal = "-"
	}

	newInitEvent := func() *zerolog.Event {
		return log.Info().Str("event", "init").Str("step_index", stepIndexVal)
	}
	newInitEventVerbose := func() *zerolog.Event {
		msg := log.Warn().Str("event", "init").Str("step_index", stepIndexVal)
		if *suppressProgress {
			msg.Discard()
		}
		return msg
	}

	var otherKeys []string
	for k := range fields {
		if k == "event" || k == "conversation_id" || k == "step_index" || k == "step_type" || k == "usage" || k == "text_delta" || k == "tools" {
			continue
		}
		otherKeys = append(otherKeys, k)
	}
	sort.Strings(otherKeys)

	for _, k := range otherKeys {
		v := fields[k]
		strVal := fmt.Sprintf("%v", v)
		if b, err := json.Marshal(v); err == nil {
			switch v.(type) {
			case map[string]any, []any:
				strVal = string(b)
			}
		}
		newInitEvent().Msg(fmt.Sprintf("%s: %s", k, strVal))
	}

	if toolsVal, exists := fields["tools"]; exists {
		var toolList []any
		switch t := toolsVal.(type) {
		case []any:
			toolList = t
		case []string:
			for _, s := range t {
				toolList = append(toolList, s)
			}
		case string:
			_ = json.Unmarshal([]byte(t), &toolList)
		}

		if len(toolList) > 0 {
			for _, tool := range toolList {
				toolName := ""
				switch t := tool.(type) {
				case string:
					toolName = t
				case map[string]any:
					if n, ok := t["name"].(string); ok {
						toolName = n
					} else {
						toolName = fmt.Sprintf("%v", t)
					}
				default:
					toolName = fmt.Sprintf("%v", tool)
				}
				if toolName != "" {
					newInitEventVerbose().Msg(fmt.Sprintf("allowed tool: %s", toolName))
				}
			}
		} else if s, ok := toolsVal.(string); ok && s != "" && len(toolList) == 0 {
			newInitEventVerbose().Msg(fmt.Sprintf("allowed tool: %s", s))
		}
	}

	if len(otherKeys) == 0 && fields["tools"] == nil {
		newInitEvent().Send()
	}
}
