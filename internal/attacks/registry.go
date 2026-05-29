package attacks

import "sort"

var registry = map[string]func() Attack{}

// Register adds a constructor for the named attack. Called from init() in each attack package.
func Register(name string, ctor func() Attack) {
	registry[name] = ctor
}

// Lookup returns a fresh Attack instance for the given name.
func Lookup(name string) (Attack, bool) {
	ctor, ok := registry[name]
	if !ok {
		return nil, false
	}
	return ctor(), true
}

// All returns one fresh instance per registered attack, sorted by name.
func All() []Attack {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Attack, 0, len(names))
	for _, n := range names {
		out = append(out, registry[n]())
	}
	return out
}
