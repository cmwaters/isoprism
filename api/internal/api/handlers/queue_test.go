package handlers

import "testing"

func TestDerivedRiskLabel(t *testing.T) {
	low := 3
	medium := 5
	high := 7

	cases := []struct {
		name  string
		score *int
		want  *string
	}{
		{name: "nil", score: nil, want: nil},
		{name: "low", score: &low, want: strPtr("low")},
		{name: "medium", score: &medium, want: strPtr("medium")},
		{name: "high", score: &high, want: strPtr("high")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := derivedRiskLabel(tc.score)
			if got == nil || tc.want == nil {
				if got != tc.want {
					t.Fatalf("derivedRiskLabel() = %v, want %v", got, tc.want)
				}
				return
			}
			if *got != *tc.want {
				t.Fatalf("derivedRiskLabel() = %q, want %q", *got, *tc.want)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
