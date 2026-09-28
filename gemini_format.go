package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

type GeminiFormatter struct {
	totalUsage         map[string]int64
	usageMu            sync.Mutex
	endOnce            sync.Once
	lastConversationID string
}

func NewGeminiFormatter() *GeminiFormatter {
	return &GeminiFormatter{
		totalUsage: make(map[string]int64),
	}
}

func (g *GeminiFormatter) End() {
	g.endOnce.Do(func() {
		g.usageMu.Lock()
		keys := make([]string, 0, len(g.totalUsage))
		for k := range g.totalUsage {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		cID := g.lastConversationID
		endLog := log.Info().Str("event", "end")
		if cID != "" {
			endLog = endLog.Str("step_index", fmt.Sprintf("%s:-", cID))
		} else {
			endLog = endLog.Str("step_index", "-")
		}
		for _, k := range keys {
			endLog = endLog.Int64(k, g.totalUsage[k])
		}
		g.usageMu.Unlock()
		endLog.Send()
	})
}

func (g *GeminiFormatter) accumulateUsage(u any) {
	if uMap, ok := u.(map[string]any); ok {
		g.usageMu.Lock()
		defer g.usageMu.Unlock()
		for k, v := range uMap {
			switch val := v.(type) {
			case float64:
				g.totalUsage[k] += int64(val)
			case int64:
				g.totalUsage[k] += val
			case int:
				g.totalUsage[k] += int64(val)
			}
		}
	}
}

func (g *GeminiFormatter) Format(map_data map[string]any) {
	eventName, _ := map_data["event"].(string)
	if eventName == "" {
		if t, ok := map_data["type"].(string); ok {
			eventName = t
		}
	}

	if u, ok := map_data["usage"]; ok {
		g.accumulateUsage(u)
		delete(map_data, "usage")
	}

	fields := make(map[string]any)
	if eventName != "" {
		fields["event"] = eventName
		if payload, ok := map_data[eventName].(map[string]any); ok {
			if u, ok := payload["usage"]; ok {
				g.accumulateUsage(u)
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
		g.usageMu.Lock()
		g.lastConversationID = c
		g.usageMu.Unlock()
	}

	if strings.EqualFold(eventName, "init") {
		g.logInitFormat(fields, convID)
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

func (g *GeminiFormatter) logInitFormat(fields map[string]any, convID string) {
	stepIndexVal := fmt.Sprintf("%s:-", convID)
	if convID == "-" {
		stepIndexVal = "-"
	}

	newInitEvent := func() *zerolog.Event {
		return log.Info().Str("event", "init").Str("step_index", stepIndexVal)
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
					newInitEvent().Msg(fmt.Sprintf("allowed tool: %s", toolName))
				}
			}
		} else if s, ok := toolsVal.(string); ok && s != "" && len(toolList) == 0 {
			newInitEvent().Msg(fmt.Sprintf("allowed tool: %s", s))
		}
	}

	if len(otherKeys) == 0 && fields["tools"] == nil {
		newInitEvent().Send()
	}
}
