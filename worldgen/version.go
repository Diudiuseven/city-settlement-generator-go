package worldgen

// DefaultAlgorithmVersion is the algorithm version used when callers
// leave AlgorithmVersion at zero.
const DefaultAlgorithmVersion uint16 = 1

// terrainFn produces the raw (pre-edit) elevation/moisture of a single
// cell. Implementations MUST be pure point functions: the value at
// (x,y) may depend only on (x,y), the seed and the parameters. That
// property is what makes chunk generation order-independent and
// seam-free, and what guarantees that rendering a chunk alone gives the
// same cells as rendering it together with its neighbors.
type terrainFn func(p Params, x, y int) (Height, Moisture)

// algorithmRegistry maps each frozen algorithm version to its point
// function. Entries are never removed and existing functions are never
// modified: old parameter combinations stay reproducible forever.
// New behavior is introduced only under a new version number.
var algorithmRegistry = map[uint16]terrainFn{}

func registerAlgorithm(version uint16, fn terrainFn) {
	if _, exists := algorithmRegistry[version]; exists {
		// Registration happens at package initialization from
		// immutable literals; a duplicate is a programming error.
		panic("worldgen: duplicate algorithm version registration")
	}
	algorithmRegistry[version] = fn
}

// terrainFor returns the frozen point function bound to a version.
func terrainFor(version uint16) (terrainFn, bool) {
	fn, ok := algorithmRegistry[version]
	return fn, ok
}
