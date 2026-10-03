package world

// Edit is an idempotent, order-independent local modification of one chunk.
//
// Semantics:
//   - An edit carries a client-chosen ID. Submitting the same ID twice is a
//     no-op (idempotency).
//   - Edits are absolute overrides of tile height/moisture. When two edits
//     touch the same tile, the edit with the lexicographically greatest ID
//     wins, regardless of submission order. Applying edits in any order, or
//     repeating submissions, therefore yields byte-identical results.
//   - After overrides are applied, derived fields (slope, biome,
//     passability) are recomputed from the modified height/moisture, so an
//     edit never leaves inconsistent tiles.
type Edit struct {
	// ID is the idempotency key. If empty, a deterministic ID is derived
	// from the edit content.
	ID string
	// Chunk is the edited chunk coordinate.
	Chunk ChunkCoord
	// Tiles maps a local tile index (ly*Size+lx) to its overrides.
	Tiles map[int]TileOverride
}

// TileOverride is an absolute override of a tile's height and/or moisture.
// A nil pointer means "leave the base value unchanged".
type TileOverride struct {
	Height   *float64
	Moisture *float64
}

// EditResult describes what an edit changed.
type EditResult struct {
	Chunk        ChunkCoord
	Applied      bool // false if the edit ID was already present
	ChangedTiles int
}
