// Package content embeds the bundled mechanics modules and scenarios.
package content

import "embed"

//go:embed modules/*.json scenarios/*.json
var FS embed.FS
