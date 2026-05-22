package governance

import (
	"fmt"
	"sync"
)

// ParameterBand describes the governance-tunable range for a single
// parameter. Soft is the "normal" band — a Simple7d proposal may
// move the parameter to any value within (SoftMin, SoftMax). Hard
// is the broader safety band — a Super* proposal may exit the soft
// band but must stay within the hard band. Constitutional proposals
// may exit even the hard band (and are the only path that can).
//
// Inclusive bounds on both ends: SoftMin ≤ v ≤ SoftMax means "within
// soft". Hard band must contain the soft band; the registry's
// register method enforces this.
//
// Spec §11 documents the actual bands per parameter:
//
//	Parameter            Soft band      Hard band
//	r*                   0.60–0.75      0.50–0.85
//	bytefee              ±50% per       ±200% per
//	max_batches_per_block 10–200        5–500
//	... etc.
//
// PLAN §7 Phase 4.3.
type ParameterBand struct {
	// Name is the canonical parameter identifier (e.g.
	// "max_batches_per_block", "c_min", "alpha"). Modules should
	// document the names they register. Names are case-sensitive.
	Name string

	// SoftMin / SoftMax bound the Simple7d-tunable range.
	SoftMin int64
	SoftMax int64

	// HardMin / HardMax bound the Super*-tunable range. Must
	// contain the soft band.
	HardMin int64
	HardMax int64
}

// ParameterRegistry is the chain-wide map of param-name →
// ParameterBand. Modules register their tunable parameters at
// genesis time; the enactment hook (Phase 4.4) consults the
// registry to validate that a proposed change is allowed given the
// proposal's class.
//
// The registry itself is in-memory state — populated at module
// init, not persisted to the state store. Genesis replay walks the
// same registration paths so determinism is preserved.
//
// Safe for concurrent reads after Register completes.
type ParameterRegistry struct {
	mu    sync.RWMutex
	bands map[string]ParameterBand
}

// NewParameterRegistry constructs an empty registry.
func NewParameterRegistry() *ParameterRegistry {
	return &ParameterRegistry{bands: make(map[string]ParameterBand)}
}

// Register adds a parameter's bands to the registry. Returns an
// error if:
//   - the name is empty
//   - soft band is malformed (SoftMin > SoftMax)
//   - hard band is malformed
//   - hard band doesn't contain the soft band
//   - the name is already registered (use Replace to override)
//
// Calling Register twice for the same name is a programmer error;
// the registry rejects rather than overwrites so a module can't
// silently shadow another's parameter.
func (r *ParameterRegistry) Register(b ParameterBand) error {
	if b.Name == "" {
		return fmt.Errorf("parameter name cannot be empty")
	}
	if b.SoftMin > b.SoftMax {
		return fmt.Errorf("parameter %q: SoftMin %d > SoftMax %d", b.Name, b.SoftMin, b.SoftMax)
	}
	if b.HardMin > b.HardMax {
		return fmt.Errorf("parameter %q: HardMin %d > HardMax %d", b.Name, b.HardMin, b.HardMax)
	}
	if b.HardMin > b.SoftMin {
		return fmt.Errorf("parameter %q: HardMin %d > SoftMin %d (hard band must contain soft)",
			b.Name, b.HardMin, b.SoftMin)
	}
	if b.HardMax < b.SoftMax {
		return fmt.Errorf("parameter %q: HardMax %d < SoftMax %d (hard band must contain soft)",
			b.Name, b.HardMax, b.SoftMax)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.bands[b.Name]; exists {
		return fmt.Errorf("parameter %q already registered (use Replace to override)", b.Name)
	}
	r.bands[b.Name] = b
	return nil
}

// Replace overwrites a registered band. Use sparingly — generally
// preferred is to fix the registration site rather than replace
// at runtime. Useful in tests.
func (r *ParameterRegistry) Replace(b ParameterBand) error {
	if b.Name == "" {
		return fmt.Errorf("parameter name cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.bands, b.Name)
	// Inline the validation here so we don't release + re-take
	// the mutex.
	if b.SoftMin > b.SoftMax {
		return fmt.Errorf("parameter %q: SoftMin %d > SoftMax %d", b.Name, b.SoftMin, b.SoftMax)
	}
	if b.HardMin > b.HardMax {
		return fmt.Errorf("parameter %q: HardMin %d > HardMax %d", b.Name, b.HardMin, b.HardMax)
	}
	if b.HardMin > b.SoftMin {
		return fmt.Errorf("parameter %q: HardMin %d > SoftMin %d (hard band must contain soft)",
			b.Name, b.HardMin, b.SoftMin)
	}
	if b.HardMax < b.SoftMax {
		return fmt.Errorf("parameter %q: HardMax %d < SoftMax %d (hard band must contain soft)",
			b.Name, b.HardMax, b.SoftMax)
	}
	r.bands[b.Name] = b
	return nil
}

// Get returns the bands for a parameter and whether the name is
// registered.
func (r *ParameterRegistry) Get(name string) (ParameterBand, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.bands[name]
	return b, ok
}

// Names returns the registered parameter names. Used by query
// endpoints and audits.
func (r *ParameterRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.bands))
	for name := range r.bands {
		out = append(out, name)
	}
	return out
}

// ValidateChange checks whether a proposed change to parameter
// `name` to value `newValue` is allowed for the given proposal
// class. Returns nil when the change is permitted; a descriptive
// error otherwise.
//
// Rules:
//   - Simple7d  : value must be within the soft band.
//   - Super30d/60d : value must be within the hard band (may
//                    exit soft).
//   - Constitutional : any value (may exit hard).
//   - Unknown parameter : rejected — every governance-tunable
//                    parameter must be in the registry. Modules
//                    that want a parameter outside this regime
//                    can implement custom validation in their
//                    enactment hook.
//
// PLAN §7 Phase 4.3.
func (r *ParameterRegistry) ValidateChange(class, name string, newValue int64) error {
	b, ok := r.Get(name)
	if !ok {
		return fmt.Errorf("parameter %q not in governance registry", name)
	}
	switch class {
	case ProposalClassConstitutional:
		// Constitutional may exit hard band — no value check.
		return nil
	case ProposalClassSuper30d, ProposalClassSuper60d:
		if newValue < b.HardMin || newValue > b.HardMax {
			return fmt.Errorf("parameter %q: value %d outside hard band [%d, %d] for class %s",
				name, newValue, b.HardMin, b.HardMax, class)
		}
		return nil
	case ProposalClassSimple7d, "":
		if newValue < b.SoftMin || newValue > b.SoftMax {
			return fmt.Errorf("parameter %q: value %d outside soft band [%d, %d] for class %s",
				name, newValue, b.SoftMin, b.SoftMax, class)
		}
		return nil
	default:
		return fmt.Errorf("unknown proposal class %q", class)
	}
}
