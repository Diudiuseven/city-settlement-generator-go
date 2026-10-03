package tests

import (
	"testing"

	"github.com/Diudiuseven/city-settlement-generator-go/world"
	"github.com/Diudiuseven/city-settlement-generator-go/worldgen"
)

// TestConnectivityAcrossSeeds: for many seeds, every settlement must be
// reachable over the road network, and road recomputation must be stable
// (calling it twice changes nothing the second time).
func TestConnectivityAcrossSeeds(t *testing.T) {
	for seed := int64(0); seed < 30; seed++ {
		w := generateWorld(t, seed, testCoords(), 4)
		if _, err := worldgen.RecomputeRoads(w); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		sets := allSettlements(w)
		if len(sets) < 2 {
			continue
		}
		// BFS from the first settlement.
		start := [2]int{int(sets[0].TileX), int(sets[0].TileY)}
		seen := map[[2]int]bool{start: true}
		queue := [][2]int{start}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, n := range roadNeighbours(w, cur[0], cur[1]) {
				if !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
		for _, s := range sets {
			if !seen[[2]int{int(s.TileX), int(s.TileY)}] {
				t.Fatalf("seed %d: settlement %d unreachable", seed, s.ID)
			}
		}
		// Stability: a second recompute must change nothing.
		before := make(map[world.ChunkCoord][]byte)
		for _, c := range testCoords() {
			before[c] = chunkBytes(t, w.GetChunk(c))
		}
		changed, err := worldgen.RecomputeRoads(w)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if len(changed) != 0 {
			t.Fatalf("seed %d: second recompute changed %v", seed, changed)
		}
		for _, c := range testCoords() {
			if string(chunkBytes(t, w.GetChunk(c))) != string(before[c]) {
				t.Fatalf("seed %d: chunk %v changed on second recompute", seed, c)
			}
		}
	}
}
