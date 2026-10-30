package model

import "strings"

// IsBotLogin reports whether a reviewer login belongs to an automated
// reviewer. Bot reviews are advisory and never count as approvals.
func IsBotLogin(login string) bool {
	return strings.HasSuffix(login, "[bot]") || strings.HasSuffix(login, "-bot") || strings.HasSuffix(login, "-reviewer")
}
