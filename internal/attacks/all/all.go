// Package all imports every attack package to trigger their init() registrations.
package all

import (
	_ "github.com/conantorreswf/limithit/internal/attacks/flood"
	_ "github.com/conantorreswf/limithit/internal/attacks/fuzz"
	_ "github.com/conantorreswf/limithit/internal/attacks/headerbomb"
	_ "github.com/conantorreswf/limithit/internal/attacks/slowloris"
	_ "github.com/conantorreswf/limithit/internal/attacks/spoof"
)
