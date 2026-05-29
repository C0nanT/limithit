package attacks

import (
	"context"
	"flag"
	"net/http"
	"time"

	"github.com/conantorreswf/limithit/internal/metrics"
)

// Report is the printable result of an attack run.
// Both *metrics.Report and *metrics.ConnReport satisfy this interface.
type Report interface {
	String() string
}

// CommonOpts carries the flags registered for every attack by runAttack.
type CommonOpts struct {
	Total       int
	Concurrency int
	Timeout     time.Duration
	Headers     http.Header
	Pacer       metrics.Pacer // nil → noop (max rate)
}

// Base carries shared, pre-built dependencies injected into Attack.Run.
type Base struct {
	URL    string
	Client *http.Client // nil for raw-socket attacks (e.g. slowloris)
	Common CommonOpts
}

// Attack is the pluggable interface each attack package must implement.
type Attack interface {
	Name() string
	Synopsis() string
	Flags(fs *flag.FlagSet) // register attack-specific flags only
	Validate() error        // called after flag.Parse; check attack-specific constraints
	Run(ctx context.Context, base Base) (Report, error)
}
