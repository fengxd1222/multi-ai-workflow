// Package harness embeds the canonical schema + default-config templates that
// `harness init` writes into a target repo's .harness/ (rev3 §2). The files
// under schemas/ are the single source of truth; they are embedded so the CLI
// ships as one self-contained binary.
package harness

import "embed"

// Templates holds schemas/*.schema.json and schemas/reserved.json.
//
//go:embed schemas
var Templates embed.FS
