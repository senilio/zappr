package model

import "testing"

func TestIsBotLogin(t *testing.T) {
	cases := map[string]bool{
		"dependabot[bot]":               true,
		"renovate-bot":                  true,
		"copilot-pull-request-reviewer": true,
		"trustly-code-reviewer":         true,
		"alice":                         false,
		"reviewer":                      false,
		"robot":                         false,
	}
	for login, want := range cases {
		if got := IsBotLogin(login); got != want {
			t.Errorf("IsBotLogin(%q) = %v, want %v", login, got, want)
		}
	}
}
