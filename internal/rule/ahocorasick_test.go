package rule

import "testing"

func TestAhoCorasickSearchKeyword(t *testing.T) {
	ac := NewAhoCorasick()
	ac.AddPattern("gambling", "block", "spam")
	ac.Build()

	result := ac.Search("this contains gambling content")
	if result == nil {
		t.Fatalf("expected a match, got nil")
	}
	if result.Pattern != "gambling" {
		t.Fatalf("expected pattern gambling, got %s", result.Pattern)
	}
	if result.Action != "block" {
		t.Fatalf("expected action block, got %s", result.Action)
	}
}

func TestAhoCorasickSearchWithRegex(t *testing.T) {
	ac := NewAhoCorasick()
	if err := ac.AddRegexPattern(`\b\d{11}\b`, "block", "privacy"); err != nil {
		t.Fatalf("failed to add regex: %v", err)
	}
	ac.Build()

	result := ac.SearchWithRegex("我的手机号是 13800138000")
	if result == nil {
		t.Fatalf("expected regex match, got nil")
	}
	if !result.IsRegex {
		t.Fatalf("expected regex result, got keyword")
	}
}

func TestSimpleMatcherCaseInsensitive(t *testing.T) {
	sm := NewSimpleMatcher()
	sm.AddKeyword("casino", "block", "gambling")

	result := sm.Match("Play CASINO tonight")
	if result == nil {
		t.Fatalf("expected a match, got nil")
	}
	if result.Pattern != "casino" {
		t.Fatalf("expected pattern casino, got %s", result.Pattern)
	}
}
