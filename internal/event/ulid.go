// Package event implements the append-only event log that is the single source
// of truth for harness state (rev3 §4). Views are rebuilt by folding events.
package event

import (
	cryptorand "crypto/rand"
	"sync"
	"time"
)

// crockford is the ULID base32 alphabet (excludes I L O U). String order of the
// 26-char encoding preserves the 128-bit value order, so a monotonic id sorts
// lexicographically — the deterministic tiebreak required by rev3 N13.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Generator produces ULIDs that are strictly monotonic within one process, so a
// single actor's event_ids never collide or go backwards (rev3 N13).
type Generator struct {
	mu      sync.Mutex
	lastMS  uint64
	entropy [10]byte
}

// NewGenerator returns a ready monotonic ULID generator.
func NewGenerator() *Generator { return &Generator{} }

// New returns the next ULID string (26 chars).
func (g *Generator) New() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	ms := uint64(time.Now().UnixMilli())
	if ms > g.lastMS {
		g.lastMS = ms
		_, _ = cryptorand.Read(g.entropy[:])
	} else {
		// Same or backward clock: keep the timestamp, increment entropy so the
		// id stays strictly increasing.
		ms = g.lastMS
		incr(g.entropy[:])
	}
	return encode(ms, g.entropy)
}

func incr(b []byte) {
	for i := len(b) - 1; i >= 0; i-- {
		b[i]++
		if b[i] != 0 {
			return
		}
	}
}

// encode packs a 48-bit timestamp + 80-bit entropy into the canonical 26-char
// ULID string (oklog/ulid bit layout).
func encode(ms uint64, ent [10]byte) string {
	var id [16]byte
	id[0] = byte(ms >> 40)
	id[1] = byte(ms >> 32)
	id[2] = byte(ms >> 24)
	id[3] = byte(ms >> 16)
	id[4] = byte(ms >> 8)
	id[5] = byte(ms)
	copy(id[6:], ent[:])

	d := make([]byte, 26)
	d[0] = crockford[(id[0]&224)>>5]
	d[1] = crockford[id[0]&31]
	d[2] = crockford[(id[1]&248)>>3]
	d[3] = crockford[((id[1]&7)<<2)|((id[2]&192)>>6)]
	d[4] = crockford[(id[2]&62)>>1]
	d[5] = crockford[((id[2]&1)<<4)|((id[3]&240)>>4)]
	d[6] = crockford[((id[3]&15)<<1)|((id[4]&128)>>7)]
	d[7] = crockford[(id[4]&124)>>2]
	d[8] = crockford[((id[4]&3)<<3)|((id[5]&224)>>5)]
	d[9] = crockford[id[5]&31]
	d[10] = crockford[(id[6]&248)>>3]
	d[11] = crockford[((id[6]&7)<<2)|((id[7]&192)>>6)]
	d[12] = crockford[(id[7]&62)>>1]
	d[13] = crockford[((id[7]&1)<<4)|((id[8]&240)>>4)]
	d[14] = crockford[((id[8]&15)<<1)|((id[9]&128)>>7)]
	d[15] = crockford[(id[9]&124)>>2]
	d[16] = crockford[((id[9]&3)<<3)|((id[10]&224)>>5)]
	d[17] = crockford[id[10]&31]
	d[18] = crockford[(id[11]&248)>>3]
	d[19] = crockford[((id[11]&7)<<2)|((id[12]&192)>>6)]
	d[20] = crockford[(id[12]&62)>>1]
	d[21] = crockford[((id[12]&1)<<4)|((id[13]&240)>>4)]
	d[22] = crockford[((id[13]&15)<<1)|((id[14]&128)>>7)]
	d[23] = crockford[(id[14]&124)>>2]
	d[24] = crockford[((id[14]&3)<<3)|((id[15]&224)>>5)]
	d[25] = crockford[id[15]&31]
	return string(d)
}
