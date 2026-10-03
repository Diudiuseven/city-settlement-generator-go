package world

// CurrentAlgorithmVersion is the version of the terrain/settlement generation
// algorithm shipped with this build. Every generated Chunk records the version
// it was produced with.
//
// Versioning rules:
//   - The algorithm of a given version is FROZEN forever: a map generated with
//     version N and identical seed+parameters must produce byte-identical
//     output on any future build.
//   - New algorithms are introduced under a new version number; old versions
//     remain selectable through Params.AlgorithmVersion.
//   - Bumping the version never silently changes an existing map.
const CurrentAlgorithmVersion = 1

// VersionHistory documents what each algorithm version introduced.
var VersionHistory = []VersionChange{
	{Version: 1, Description: "initial release: hash-based value-noise terrain, Whittaker-style biomes, deterministic settlement placement, MST+A* road network"},
}

// VersionChange describes one algorithm version.
type VersionChange struct {
	Version     int
	Description string
}
