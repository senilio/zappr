package model

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// Key identifies a native comment across edits. Legacy content keys are only
// suitable for viewing; they are not evidence for assembling report fragments.
func (c PRComment) Key() string {
	kind := c.Kind
	if kind == "" {
		kind = "comment"
	}
	if c.ID != "" {
		return kind + ":" + c.ID
	}
	c.ID, c.URL = "", ""
	c.UpdatedAt = c.CreatedAt
	b, _ := json.Marshal(c)
	return fmt.Sprintf("%s:legacy:%x", kind, sha256.Sum256(b))
}
