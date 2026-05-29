package metrics

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"
)

type Pacer interface {
	Next() time.Duration
}

type noopPacer struct{}

func (noopPacer) Next() time.Duration { return 0 }

func NoopPacer() Pacer { return noopPacer{} }

type uniformPacer struct {
	min, max time.Duration
	rng      *rand.Rand
}

func (u *uniformPacer) Next() time.Duration {
	if u.max <= u.min {
		return u.min
	}
	span := int64(u.max - u.min)
	return u.min + time.Duration(u.rng.Int64N(span))
}

type poissonPacer struct {
	mean float64
	rng  *rand.Rand
}

func (p *poissonPacer) Next() time.Duration {
	u := p.rng.Float64()
	if u <= 0 {
		u = 1e-12
	}
	d := -math.Log(u) * p.mean
	return time.Duration(d)
}

type zipfPacer struct {
	min, max time.Duration
	z        *rand.Zipf
}

func (z *zipfPacer) Next() time.Duration {
	n := z.z.Uint64()
	span := int64(z.max - z.min)
	if span <= 0 {
		return z.min
	}
	delta := int64(n) % span
	return z.min + time.Duration(delta)
}

type rampPacer struct {
	startRate float64
	endRate   float64
	rampDur   time.Duration
	startedAt time.Time
}

func (r *rampPacer) Next() time.Duration {
	elapsed := time.Since(r.startedAt)
	var rate float64
	if r.rampDur <= 0 || elapsed >= r.rampDur {
		rate = r.endRate
	} else {
		frac := float64(elapsed) / float64(r.rampDur)
		rate = r.startRate + (r.endRate-r.startRate)*frac
	}
	if rate <= 0 {
		return 0
	}
	return time.Duration(float64(time.Second) / rate)
}

// NewRampPacer returns a Pacer that linearly increases from startRPS to endRPS over rampDuration.
// After rampDuration elapses, rate is fixed at endRPS.
func NewRampPacer(startRPS, endRPS float64, rampDuration time.Duration) (Pacer, error) {
	if endRPS <= 0 {
		return nil, errors.New("ramp pacer: endRPS must be > 0")
	}
	if startRPS < 0 {
		return nil, errors.New("ramp pacer: startRPS must be >= 0")
	}
	return &rampPacer{
		startRate: startRPS,
		endRate:   endRPS,
		rampDur:   rampDuration,
		startedAt: time.Now(),
	}, nil
}

func NewPacer(kind string, minDelay, maxDelay time.Duration, rps float64) (Pacer, error) {
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	switch strings.ToLower(kind) {
	case "", "none":
		return NoopPacer(), nil
	case "uniform":
		if minDelay < 0 || maxDelay < minDelay {
			return nil, errors.New("uniform pacer: invalid min/max")
		}
		return &uniformPacer{min: minDelay, max: maxDelay, rng: rng}, nil
	case "poisson":
		if rps <= 0 {
			return nil, errors.New("poisson pacer requires rps > 0")
		}
		mean := float64(time.Second) / rps
		return &poissonPacer{mean: mean, rng: rng}, nil
	case "zipf":
		if minDelay < 0 || maxDelay <= minDelay {
			return nil, errors.New("zipf pacer: invalid min/max")
		}
		// s>1, v>=1, imax large enough to give long tail
		z := rand.NewZipf(rng, 1.2, 1.0, 1<<20)
		if z == nil {
			return nil, errors.New("zipf pacer init failed")
		}
		return &zipfPacer{min: minDelay, max: maxDelay, z: z}, nil
	default:
		return nil, fmt.Errorf("unknown pacer %q (uniform|poisson|zipf|none)", kind)
	}
}
