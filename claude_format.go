package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

type ClaudeFormatter struct {
	totalUsage    map[string]int64
	usageMu       sync.Mutex
	endOnce       sync.Once
	lastSessionID string
	stepCounter   int64
}

func NewClaudeFormatter() *ClaudeFormatter {
	return &ClaudeFormatter{
		totalUsage: make(map[string]int64),
	}
}

func (c *ClaudeFormatter) End() {
	c.endOnce.Do(func() {
		c.usageMu.Lock()
		keys := make([]string, 0, len(c.totalUsage))
		for k := range c.totalUsage {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sID := c.lastSessionID
		endLog := log.Info().Str("event", "end")
		if sID != "" {
			endLog = endLog.Str("step_index", fmt.Sprintf("%s:-", sID))
		} else {
			endLog = endLog.Str("step_index", "-")
		}
		for _, k := range keys {
			endLog = endLog.Int64(k, c.totalUsage[k])
		}
		c.usageMu.Unlock()
		endLog.Send()
	})
}

func (c *ClaudeFormatter) accumulateUsage(u any) {
	if uMap, ok := u.(map[string]any); ok {
		c.usageMu.Lock()
		defer c.usageMu.Unlock()
		var inputTokens, outputTokens int64
		for k, v := range uMap {
			var val int64
			switch vt := v.(type) {
			case float64:
				val = int64(vt)
			case int64:
				val = vt
			case int:
				val = int64(vt)
			}
			c.totalUsage[k] += val
			if k == "input_tokens" {
				inputTokens = val
			} else if k == "output_tokens" {
				outputTokens = val
			}
		}
		if _, hasTotal := uMap["total_tokens"]; !hasTotal && (inputTokens > 0 || outputTokens > 0) {
			c.totalUsage["total_tokens"] += (inputTokens + outputTokens)
		}
	}
}

func (c *ClaudeFormatter) Format(map_data map[string]any) {
	eventType, _ := map_data["type"].(string)
	subtype, _ := map_data["subtype"].(string)

	// Extract session_id or conversation_id
	sessID := "-"
	if s, ok := map_data["session_id"].(string); ok && s != "" {
		sessID = s
	} else if cid, ok := map_data["conversation_id"].(string); ok && cid != "" {
		sessID = cid
	}

	if sessID != "-" {
		c.usageMu.Lock()
		c.lastSessionID = sessID
		c.usageMu.Unlock()
	} else {
		c.usageMu.Lock()
		if c.lastSessionID != "" {
			sessID = c.lastSessionID
		}
		c.usageMu.Unlock()
	}

	// Accumulate usage if present
	if u, ok := map_data["usage"]; ok {
		c.accumulateUsage(u)
		delete(map_data, "usage")
	}
	if msg, ok := map_data["message"].(map[string]any); ok {
		if u, ok := msg["usage"]; ok {
			c.accumulateUsage(u)
			delete(msg, "usage")
		}
	}

	// 1. Handle system/init event
	if (strings.EqualFold(eventType, "system") && strings.EqualFold(subtype, "init")) ||
		strings.EqualFold(eventType, "init") ||
		strings.EqualFold(subtype, "init") {
		c.logInitFormat(map_data, sessID)
		return
	}

	// 2. Handle result event
	if strings.EqualFold(eventType, "result") {
		c.logResultFormat(map_data, sessID)
		return
	}

	// 3. Handle error event
	if strings.EqualFold(eventType, "error") {
		c.logErrorFormat(map_data, sessID)
		return
	}

	// 4. Handle assistant, tool_use, tool_result, user, content_block_delta, etc.
	c.logStepFormat(map_data, eventType, subtype, sessID)
}

func (c *ClaudeFormatter) logInitFormat(map_data map[string]any, sessID string) {
	stepIndexVal := fmt.Sprintf("%s:-", sessID)
	if sessID == "-" {
		stepIndexVal = "-"
	}

	newInitEvent := func() *zerolog.Event {
		return log.Info().Str("event", "init").Str("step_index", stepIndexVal)
	}

	fields := make(map[string]any)
	for k, v := range map_data {
		if k == "type" || k == "subtype" || k == "session_id" || k == "conversation_id" || k == "usage" || k == "tools" {
			continue
		}
		fields[k] = v
	}

	var otherKeys []string
	for k := range fields {
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

	if toolsVal, exists := map_data["tools"]; exists {
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
		}
	}

	if len(otherKeys) == 0 && map_data["tools"] == nil {
		newInitEvent().Send()
	}
}

func (c *ClaudeFormatter) logResultFormat(map_data map[string]any, sessID string) {
	stepIndexVal := fmt.Sprintf("%s:-", sessID)
	if sessID == "-" {
		stepIndexVal = "-"
	}

	subtype, _ := map_data["subtype"].(string)
	isError, _ := map_data["is_error"].(bool)
	if strings.EqualFold(subtype, "error") {
		isError = true
	}

	var resLog *zerolog.Event
	if isError {
		resLog = log.Error()
	} else {
		resLog = log.Info()
	}

	resLog = resLog.Str("event", "result").Str("step_index", stepIndexVal)

	if subtype != "" {
		resLog = resLog.Str("status", strings.ToUpper(subtype))
	} else if isError {
		resLog = resLog.Str("status", "ERROR")
	} else {
		resLog = resLog.Str("status", "SUCCESS")
	}

	if dMs, ok := map_data["duration_ms"].(float64); ok {
		resLog = resLog.Float64("duration_seconds", dMs/1000.0)
	} else if dSec, ok := map_data["duration_seconds"].(float64); ok {
		resLog = resLog.Float64("duration_seconds", dSec)
	}

	if cost, ok := map_data["cost_usd"].(float64); ok {
		resLog = resLog.Float64("cost_usd", cost)
	}

	if turns, ok := map_data["num_turns"].(float64); ok {
		resLog = resLog.Int64("num_turns", int64(turns))
	}

	for k, v := range map_data {
		if k == "type" || k == "subtype" || k == "session_id" || k == "conversation_id" ||
			k == "usage" || k == "duration_ms" || k == "duration_seconds" || k == "cost_usd" ||
			k == "num_turns" || k == "is_error" {
			continue
		}
		switch val := v.(type) {
		case string:
			resLog = resLog.Str(k, val)
		case float64:
			if val == float64(int64(val)) {
				resLog = resLog.Int64(k, int64(val))
			} else {
				resLog = resLog.Float64(k, val)
			}
		case bool:
			resLog = resLog.Bool(k, val)
		default:
			if b, err := json.Marshal(val); err == nil {
				resLog = resLog.Str(k, string(b))
			} else {
				resLog = resLog.Str(k, fmt.Sprintf("%v", val))
			}
		}
	}

	resLog.Send()
}

func (c *ClaudeFormatter) logErrorFormat(map_data map[string]any, sessID string) {
	stepIndexVal := fmt.Sprintf("%s:-", sessID)
	if sessID == "-" {
		stepIndexVal = "-"
	}

	errLog := log.Error().Str("event", "error").Str("step_index", stepIndexVal)
	if errObj, ok := map_data["error"].(map[string]any); ok {
		if msg, ok := errObj["message"].(string); ok {
			errLog = errLog.Str("message", msg)
		}
	} else if msg, ok := map_data["message"].(string); ok {
		errLog = errLog.Str("message", msg)
	}

	for k, v := range map_data {
		if k == "type" || k == "subtype" || k == "session_id" || k == "conversation_id" || k == "error" || k == "message" || k == "usage" {
			continue
		}
		errLog = errLog.Str(k, fmt.Sprintf("%v", v))
	}
	errLog.Send()
}

func (c *ClaudeFormatter) logStepFormat(map_data map[string]any, eventType, subtype, sessID string) {
	// Determine step index
	stepIdx := "-"
	if si, ok := map_data["step_index"]; ok {
		if f, ok := si.(float64); ok {
			stepIdx = fmt.Sprintf("%d", int64(f))
		} else {
			stepIdx = fmt.Sprintf("%v", si)
		}
	} else if idx, ok := map_data["index"]; ok {
		if f, ok := idx.(float64); ok {
			stepIdx = fmt.Sprintf("%d", int64(f))
		} else {
			stepIdx = fmt.Sprintf("%v", idx)
		}
	} else {
		c.stepCounter++
		stepIdx = fmt.Sprintf("%d", c.stepCounter)
	}

	stepIndexVal := "-"
	if sessID != "-" || stepIdx != "-" {
		stepIndexVal = fmt.Sprintf("%s:%s", sessID, stepIdx)
	}

	// Classify event label
	eventLabel := "STEP_UPDATE"
	if eventType != "" {
		eventLabel = strings.ToUpper(eventType)
	}
	if strings.EqualFold(eventType, "assistant") {
		eventLabel = "AGENT_RESPONSE"
	} else if strings.EqualFold(eventType, "user") {
		eventLabel = "USER_INPUT"
	} else if strings.EqualFold(eventType, "content_block_delta") {
		eventLabel = "AGENT_RESPONSE"
	}

	// Extract text delta if available
	textDelta := ""
	if td, ok := map_data["text_delta"].(string); ok && td != "" {
		textDelta = td
	} else if delta, ok := map_data["delta"].(map[string]any); ok {
		if t, ok := delta["text"].(string); ok && t != "" {
			textDelta = t
		} else if td, ok := delta["text_delta"].(string); ok && td != "" {
			textDelta = td
		}
	}

	// Check if message content has tool_use or text
	if msg, ok := map_data["message"].(map[string]any); ok {
		if contentList, ok := msg["content"].([]any); ok {
			for _, item := range contentList {
				if itemMap, ok := item.(map[string]any); ok {
					itemType, _ := itemMap["type"].(string)
					if strings.EqualFold(itemType, "tool_use") {
						eventLabel = "TOOL_USE"
						if name, ok := itemMap["name"].(string); ok {
							map_data["tool_name"] = name
						}
					} else if strings.EqualFold(itemType, "text") && textDelta == "" {
						if txt, ok := itemMap["text"].(string); ok {
							textDelta = txt
						}
					}
				}
			}
		}
	}

	// Check for tool_use at top level
	if strings.EqualFold(eventType, "tool_use") {
		eventLabel = "TOOL_USE"
	} else if strings.EqualFold(eventType, "tool_result") {
		eventLabel = "TOOL_RESULT"
	}

	eventLog := log.Warn().Str("event", eventLabel).Str("step_index", stepIndexVal)
	if textDelta != "" {
		eventLog = eventLog.Str("text_delta", textDelta)
	}

	for k, v := range map_data {
		if k == "type" || k == "subtype" || k == "session_id" || k == "conversation_id" ||
			k == "step_index" || k == "index" || k == "usage" || k == "text_delta" ||
			k == "delta" || k == "message" {
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
