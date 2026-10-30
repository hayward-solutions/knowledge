package content

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

func MentionedUserIDs(contentJSON []byte) []uuid.UUID {
	out := []uuid.UUID{}
	trimmed := strings.TrimSpace(string(contentJSON))
	if trimmed == "" || trimmed == "null" {
		return out
	}
	var root pmNode
	if err := json.Unmarshal([]byte(trimmed), &root); err != nil {
		return out
	}
	seen := map[uuid.UUID]bool{}
	var walk func(n pmNode)
	walk = func(n pmNode) {
		if n.Type == "mention" {
			if raw, ok := n.Attrs["id"].(string); ok {
				if id, err := uuid.Parse(raw); err == nil && !seen[id] {
					seen[id] = true
					out = append(out, id)
				}
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(root)
	return out
}
