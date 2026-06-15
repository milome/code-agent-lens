package dump

import (
	"encoding/json"
	"fmt"
	"strings"
)

func ExtractPrompts(clientFormat string, body []byte) PromptIndex {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return PromptIndex{}
	}
	var out PromptIndex
	format := strings.ToLower(strings.TrimSpace(clientFormat))
	switch format {
	case "claude":
		appendPrompt(&out, "system", payload["system"])
		appendMessages(&out, payload["messages"])
	case "openai_chat":
		appendMessages(&out, payload["messages"])
	case "openai_responses":
		appendPrompt(&out, "system", payload["instructions"])
		appendMessages(&out, payload["input"])
	default:
		appendPrompt(&out, "system", payload["system"])
		appendMessages(&out, payload["messages"])
		appendMessages(&out, payload["input"])
	}
	return out
}

func appendMessages(out *PromptIndex, value any) {
	items, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if strings.TrimSpace(role) == "" {
			role = "unknown"
		}
		appendPrompt(out, role, msg["content"])
	}
}

func appendPrompt(out *PromptIndex, role string, value any) {
	text := extractText(value)
	if strings.TrimSpace(text) == "" {
		return
	}
	out.Prompts = append(out.Prompts, PromptRecord{Role: strings.TrimSpace(role), Text: text})
}

func extractText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text := extractText(item); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		for _, key := range []string{"text", "input_text", "content"} {
			if text := extractText(v[key]); strings.TrimSpace(text) != "" {
				return text
			}
		}
	default:
		return fmt.Sprint(v)
	}
	return ""
}
