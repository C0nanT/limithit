package attacks_test

import (
	"context"
	"flag"
	"testing"

	"github.com/conantorreswf/limithit/internal/attacks"
)

type stubAttack struct{ name string }

func (s *stubAttack) Name() string          { return s.name }
func (s *stubAttack) Synopsis() string      { return "stub" }
func (s *stubAttack) Flags(_ *flag.FlagSet) {}
func (s *stubAttack) Validate() error       { return nil }
func (s *stubAttack) Run(_ context.Context, _ attacks.Base) (attacks.Report, error) {
	return nil, nil
}

func TestRegistryLookup(t *testing.T) {
	attacks.Register("zzz-stub-a", func() attacks.Attack { return &stubAttack{"zzz-stub-a"} })
	attacks.Register("zzz-stub-b", func() attacks.Attack { return &stubAttack{"zzz-stub-b"} })

	a, ok := attacks.Lookup("zzz-stub-a")
	if !ok || a.Name() != "zzz-stub-a" {
		t.Fatal("Lookup zzz-stub-a failed")
	}
	_, ok = attacks.Lookup("no-such-attack")
	if ok {
		t.Fatal("Lookup should return false for unknown attack")
	}
}

func TestRegistryAllSorted(t *testing.T) {
	attacks.Register("zzz-stub-c", func() attacks.Attack { return &stubAttack{"zzz-stub-c"} })

	all := attacks.All()
	for i := 1; i < len(all); i++ {
		if all[i-1].Name() >= all[i].Name() {
			t.Fatalf("All() not sorted: %q >= %q", all[i-1].Name(), all[i].Name())
		}
	}
}

func TestRegistryLookupReturnsFreshInstance(t *testing.T) {
	attacks.Register("zzz-stub-d", func() attacks.Attack { return &stubAttack{"zzz-stub-d"} })

	a1, _ := attacks.Lookup("zzz-stub-d")
	a2, _ := attacks.Lookup("zzz-stub-d")
	if a1 == a2 {
		t.Fatal("Lookup should return distinct instances")
	}
}
