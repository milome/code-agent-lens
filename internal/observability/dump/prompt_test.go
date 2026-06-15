package dump

import "testing"

func TestExtractPromptsFromClaude(t *testing.T) {
	body := []byte(`{
		"system":[{"type":"text","text":"system prompt"}],
		"messages":[
			{"role":"user","content":[{"type":"text","text":"hello"},{"type":"image","source":{"type":"base64","data":"abc"}}]},
			{"role":"assistant","content":"assistant text"}
		]
	}`)
	idx := ExtractPrompts("claude", body)
	assertPrompt(t, idx, "system", "system prompt")
	assertPrompt(t, idx, "user", "hello")
	assertPrompt(t, idx, "assistant", "assistant text")
}

func TestExtractPromptsFromOpenAIChat(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"developer","content":"dev prompt"},
		{"role":"system","content":"system prompt"},
		{"role":"user","content":[{"type":"text","text":"user text"}]}
	]}`)
	idx := ExtractPrompts("openai_chat", body)
	assertPrompt(t, idx, "developer", "dev prompt")
	assertPrompt(t, idx, "system", "system prompt")
	assertPrompt(t, idx, "user", "user text")
}

func TestExtractPromptsFromOpenAIResponses(t *testing.T) {
	body := []byte(`{
		"instructions":"system instructions",
		"input":[
			{"role":"developer","content":[{"type":"input_text","text":"dev text"}]},
			{"role":"user","content":"user text"}
		]
	}`)
	idx := ExtractPrompts("openai_responses", body)
	assertPrompt(t, idx, "system", "system instructions")
	assertPrompt(t, idx, "developer", "dev text")
	assertPrompt(t, idx, "user", "user text")
}

func TestExtractPromptsInvalidOrEmpty(t *testing.T) {
	if idx := ExtractPrompts("openai_chat", []byte(`{`)); len(idx.Prompts) != 0 {
		t.Fatalf("invalid json prompts = %+v", idx.Prompts)
	}
	if idx := ExtractPrompts("openai_chat", []byte(`{"metadata":true}`)); len(idx.Prompts) != 0 {
		t.Fatalf("no prompt prompts = %+v", idx.Prompts)
	}
}

func assertPrompt(t *testing.T, idx PromptIndex, role string, contains string) {
	t.Helper()
	for _, p := range idx.Prompts {
		if p.Role == role && p.Text == contains {
			return
		}
	}
	t.Fatalf("missing role=%s text=%q in %+v", role, contains, idx.Prompts)
}
