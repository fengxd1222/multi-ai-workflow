package cli

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// genID builds a readable, sortable, collision-resistant id: PREFIX-<utc>-<rand>.
func genID(prefix string) string {
	var b [2]byte
	_, _ = cryptorand.Read(b[:])
	return fmt.Sprintf("%s-%s-%s", prefix, time.Now().UTC().Format("20060102-150405"), hex.EncodeToString(b[:]))
}

func newJobID() string  { return genID("J") }
func newGateID() string { return genID("G") }
func newTaskID() string { return genID("T") }
