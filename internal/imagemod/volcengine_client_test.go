package imagemod

import "testing"

func TestConvertToInternalResultActionMapping(t *testing.T) {
	client := &VolcengineImageModerationClient{}
	input := &ModerationResult{
		RequestId: "req-1",
		Data: Data{
			Antispam: Antispam{
				Suggestion: "block",
				Label:      "porn",
				Rate:       0.92,
			},
		},
	}

	out := client.ConvertToInternalResult(input)
	if out.Action != "block" {
		t.Fatalf("expected action block, got %s", out.Action)
	}
	if out.RiskScore != 0.92 {
		t.Fatalf("expected risk score 0.92, got %f", out.RiskScore)
	}
	if len(out.Labels) == 0 || out.Labels[0].Name != "porn" {
		t.Fatalf("expected porn label, got %+v", out.Labels)
	}
}

func TestGenerateReasonWithLowConfidenceLabels(t *testing.T) {
	client := &VolcengineImageModerationClient{}
	reason := client.generateReason([]Label{
		{Name: "test", Confidence: 0.2},
	}, 0.1)
	if reason == "" {
		t.Fatalf("expected non-empty reason")
	}
}
