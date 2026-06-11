package pipeline

import "testing"

func TestTranslationStatusFromRecovery(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   TranslationStatus
	}{
		{name: "success", status: "Success", want: TranslationStatusSuccess},
		{name: "partial_success", status: "Partial Success", want: TranslationStatusPartialSuccess},
		{name: "failure", status: "Failure", want: TranslationStatusFailure},
		{name: "unknown", status: "Unknown", want: TranslationStatusFailure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := translationStatusFromRecovery(tc.status)
			if got != tc.want {
				t.Fatalf("translationStatusFromRecovery(%q) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}
