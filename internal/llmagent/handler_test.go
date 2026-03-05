package llmagent

import "testing"

func TestCleanJSONWithMarkdownFence(t *testing.T) {
	input := "```json\n{\"action\":\"allow\",\"reason\":\"ok\"}\n```"
	output := cleanJSON(input)
	expected := "{\"action\":\"allow\",\"reason\":\"ok\"}"
	if output != expected {
		t.Fatalf("expected %s, got %s", expected, output)
	}
}

func TestCleanJSONWithoutFence(t *testing.T) {
	input := "{\"action\":\"review\",\"reason\":\"need check\"}"
	output := cleanJSON(input)
	if output != input {
		t.Fatalf("expected unchanged json, got %s", output)
	}
}
