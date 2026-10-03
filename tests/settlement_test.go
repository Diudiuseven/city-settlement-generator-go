package tests

import (
	"math"
	"testing"

	"github.com/Diudiuseven/city-settlement-generator-go/world"
)

// allSettlements collects every settlement in the resident chunks.
func allSettlements(w *world.World) []world.Settlement {
	var out []world.Settlement
	for _, c := range w.ChunkCoords() {
		ch := w.GetChunk(c)
		out = append(out, ch.Settlements...)
	}
	return out
}

// TestSettlementSpacing: settlements never overlap and respect minimum
// spacing, including across chunk borders.
func TestSettlementSpacing(t *testing.T) {
	w := generateWorld(t, 555, testCoords(), 4)
	sets := allSettlements(w)
	minDist := w.Params().Settlement.MinSpacing
	for i := 0; i < len(sets); i++ {
		for j := i + 1; j < len(sets); j++ {
			dx := float64(sets[i].TileX - sets[j].TileX)
			dy := float64(sets[i].TileY - sets[j].TileY)
			d := math.Hypot(dx, dy)
			if d < minDist-0.5 {
				t.Fatalf("settlements %d and %d are too close: %.2f < %.2f", sets[i].ID, sets[j].ID, d, minDist)
			}
		}
	}
}

// TestSettlementStability: regenerating the world yields the exact same
// settlements.
func TestSettlementStability(t *testing.T) {
	w1 := generateWorld(t, 8080, testCoords(), 4)
	w2 := generateWorld(t, 8080, testCoords(), 1)
	s1 := allSettlements(w1)
	s2 := allSettlements(w2)
	if len(s1) != len(s2) {
		t.Fatalf("settlement count differs: %d vs %d", len(s1), len(s2))
	}
	for i := range s1 {
		if s1[i].ID != s2[i].ID || s1[i].TileX != s2[i].TileX || s1[i].TileY != s2[i].TileY {
			t.Fatalf("settlement %d differs: (%d,%d) vs (%d,%d)", i,
				s1[i].TileX, s1[i].TileY, s2[i].TileX, s2[i].TileY)
		}
	}
}

// TestSettlementSuitability: settlements never sit on water, cliffs or snow.
func TestSettlementSuitability(t *testing.T) {
	w := generateWorld(t, 1212, testCoords(), 4)
	for _, s := range allSettlements(w) {
		ch := w.GetChunk(world.ChunkCoord{X: int(s.ChunkX), Y: int(s.ChunkY)})
		tile := ch.GlobalTile(int(s.TileX), int(s.TileY))
		if tile == nil {
			t.Fatalf("settlement %d tile not found", s.ID)
		}
		if tile.IsWater() {
			t.Fatalf("settlement %d is on water", s.ID)
		}
		if tile.IsImpassable() {
			t.Fatalf("settlement %d is on impassable terrain", s.ID)
		}
		if tile.Biome == world.BiomeSnow {
			t.Fatalf("settlement %d is on snow", s.ID)
		}
		if tile.Slope > w.Params().Settlement.MaxSlope+1e-9 {
			t.Fatalf("settlement %d slope %.3f exceeds max %.3f", s.ID, tile.Slope, w.Params().Settlement.MaxSlope)
		}
	}
}

// TestSettlementsHaveNames: every settlement gets a deterministic name.
func TestSettlementsHaveNames(t *testing.T) {
	w := generateWorld(t, 33, testCoords(), 4)
	for _, s := range allSettlements(w) {
		if s.Name == "" {
			t.Fatalf("settlement %d has no name", s.ID)
		}
	}
}
