package resource

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// DefinitionRevision preserves configuration independently of editor CAS.
type DefinitionRevision struct {
	ID         string     `json:"id"`
	Number     int64      `json:"number"`
	Definition Definition `json:"definition"`
	CreatedAt  time.Time  `json:"createdAt"`
	Archived   bool       `json:"archived"`
}

func NewDefinitionRevision(d Definition, number int64) DefinitionRevision {
	d.RevisionID = ""
	raw, _ := json.Marshal(struct {
		Number     int64
		Definition Definition
	}{number, d})
	sum := sha256.Sum256(raw)
	id := hex.EncodeToString(sum[:])
	d.RevisionID = id
	return DefinitionRevision{ID: id, Number: number, Definition: d, CreatedAt: time.Now().UTC()}
}
