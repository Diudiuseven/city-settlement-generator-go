package tests

import (
	"testing"

	"github.com/Diudiuseven/city-settlement-generator-go/world"
	"github.com/Diudiuseven/city-settlement-generator-go/worldgen"
)

// roadNeighbours returns the 8 neighbours of a road tile that carry a road.
func roadNeighbours(w *world.World, tx, ty int) [][2]int {
	var out [][2]int
	for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		nx, ny := tx+d[0], ty+d[1]
		ch := w.GetChunk(world.ChunkCoord{X: floorDiv(nx, w.ChunkSize()), Y: floorDiv(ny, w.ChunkSize())})
		if ch == nil {
			continue
		}
		t := ch.GlobalTile(nx, ny)
		if t != nil && t.HasRoad() {
			out = append(out, [2]int{nx, ny})
		}
	}
	return out
}

func floorDiv(a, b int) int {
	q := a / b
	if a < 0 && a%b != 0 {
		q--
	}
	return q
}

// TestRoadsConnectAllSettlements: after road construction every settlement is
// reachable from every other over the road network.
func TestRoadsConnectAllSettlements(t *testing.T) {
	w := generateWorld(t, 4242, testCoords(), 4)
	if _, err := worldgen.RecomputeRoads(w); err != nil {
		t.Fatal(err)
	}
	sets := allSettlements(w)
	if len(sets) < 2 {
		t.Skip("not enough settlements to test connectivity")
	}
	// BFS from the first settlement over road tiles.
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
			t.Fatalf("settlement %d at (%d,%d) is unreachable over roads", s.ID, s.TileX, s.TileY)
		}
	}
}

// TestRoadsAvoidImpassableTerrain: no road tile may be impassable.
func TestRoadsAvoidImpassableTerrain(t *testing.T) {
	w := generateWorld(t, 777, testCoords(), 4)
	if _, err := worldgen.RecomputeRoads(w); err != nil {
		t.Fatal(err)
	}
	for _, c := range w.ChunkCoords() {
		ch := w.GetChunk(c)
		for ly := 0; ly < ch.Size; ly++ {
			for lx := 0; lx < ch.Size; lx++ {
				tile := ch.LocalTile(lx, ly)
				if tile.HasRoad() && tile.IsImpassable() {
					t.Fatalf("road crosses impassable tile at chunk %v (%d,%d)", c, lx, ly)
				}
			}
		}
	}
}

// TestRoadsAreDeterministic: rebuilding roads twice yields identical bytes.
func TestRoadsAreDeterministic(t *testing.T) {
	coords := testCoords()
	w1 := generateWorld(t, 9090, coords, 4)
	w2 := generateWorld(t, 9090, coords, 4)
	if _, err := worldgen.RecomputeRoads(w1); err != nil {
		t.Fatal(err)
	}
	if _, err := worldgen.RecomputeRoads(w2); err != nil {
		t.Fatal(err)
	}
	for _, c := range coords {
		if string(chunkBytes(t, w1.GetChunk(c))) != string(chunkBytes(t, w2.GetChunk(c))) {
			t.Fatalf("chunk %v differs after identical road recomputation", c)
		}
	}
}

// TestRoadsDoNotTouchTerrainElsewhere: road parameters never change terrain.
func TestRoadParamsDoNotChangeTerrain(t *testing.T) {
	p1 := world.DefaultParams()
	p2 := world.DefaultParams()
	p2.RoadParams.LandCost = 99.0
	p2.RoadParams.WaterCost = 0.1
	p2.RoadParams.MaxSlope = 0.01

	w1, err := world.New(11, p1)
	if err != nil {
		t.Fatal(err)
	}
	w2, err := world.New(11, p2)
	if err != nil {
		t.Fatal(err)
	}
	coords := testCoords()
	for _, c := range coords {
		ch1, err := worldgen.GenerateChunk(w1, c)
		if err != nil {
			t.Fatal(err)
		}
		ch2, err := worldgen.GenerateChunk(w2, c)
		if err != nil {
			t.Fatal(err)
		}
		if string(chunkBytes(t, ch1)) != string(chunkBytes(t, ch2)) {
			t.Fatalf("road parameters changed terrain of chunk %v", c)
		}
	}
}
