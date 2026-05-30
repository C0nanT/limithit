// Package all imports every attack package to trigger their init() registrations.
package all

import (
	_ "github.com/conantorreswf/limithit/internal/attacks/flood"       // register flood
	_ "github.com/conantorreswf/limithit/internal/attacks/fuzz"        // register fuzz
	_ "github.com/conantorreswf/limithit/internal/attacks/gzipbomb"    // register gzipbomb
	_ "github.com/conantorreswf/limithit/internal/attacks/h2flood"     // register h2flood
	_ "github.com/conantorreswf/limithit/internal/attacks/headerbomb"  // register headerbomb
	_ "github.com/conantorreswf/limithit/internal/attacks/methodspray" // register methodspray
	_ "github.com/conantorreswf/limithit/internal/attacks/replay"      // register replay
	_ "github.com/conantorreswf/limithit/internal/attacks/slowloris"   // register slowloris
	_ "github.com/conantorreswf/limithit/internal/attacks/spoof"       // register spoof
	_ "github.com/conantorreswf/limithit/internal/attacks/wsflood"     // register wsflood
)
