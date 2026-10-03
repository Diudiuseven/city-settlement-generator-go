package tests

import (
	"testing"

	"github.com/Diudiuseven/city-settlement-generator-go/world"
	"github.com/Diudiuseven/city-settlement-generator-go/worldgen"
)

// TestVersionRecorded: every chunk records the algorithm version it was
// produced with.
func TestVersionRecorded(t *testing.T) {
	w := generateWorld(t, 1, testCoords(), 4)
	for _, c := range testCoords() {
		ch := w.GetChunk(c)
		if ch.AlgorithmVersion != world.CurrentAlgorithmVersion {
			t.Fatalf("chunk %v version %d, want %d", c, ch.AlgorithmVersion, world.CurrentAlgorithmVersion)
		}
		if ch.Seed != 1 {
			t.Fatalf("chunk %v seed %d, want 1", c, ch.Seed)
		}
	}
}

// TestExplicitVersion: selecting version 1 explicitly works and records it.
func TestExplicitVersion(t *testing.T) {
	p := testParams()
	p.AlgorithmVersion = 1
	w, err := world.New(2, p)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := worldgen.GenerateChunk(w, world.ChunkCoord{X: 0, Y: 0})
	if err != nil {
		t.Fatal(err)
	}
	if ch.AlgorithmVersion != 1 {
		t.Fatalf("got version %d", ch.AlgorithmVersion)
	}
}

// TestNonTerrainParamsDoNotChangeTerrain: road parameters never affect
// terrain, moisture, biomes or settlements.
func TestNonTerrainParamsDoNotChangeTerrain(t *testing.T) {
	p1 := testParams()
	p2 := testParams()
	p2.RoadParams.LandCost = 99.0
	p2.RoadParams.WaterCost = 0.1
	p2.RoadParams.MaxSlope = 0.01
	p2.Settlement.MinSpacing = 999 // settlement params DO change settlements;
	// keep them equal to isolate road-parameter effects.
	p2.Settlement = p1.Settlement

	w1, err := world.New(11, p1)
	if err != nil {
		t.Fatal(err)
	}
	w2, err := world.New(11, p2)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range testCoords() {
		ch1, err := worldgen.GenerateChunk(w1, c)
		if err != nil {
			t.Fatal(err)
		}
		ch2, err := worldgen.GenerateChunk(w2, c)
		if err != nil {
			t.Fatal(err)
		}
		if string(chunkBytes(t, ch1)) != string(chunkBytes(t, ch2)) {
			t.Fatalf("road parameters changed chunk %v", c)
		}
	}
}

// TestTerrainParamsChangeMap: changing a terrain parameter does change the
// map (sanity that parameters are wired in).
func TestTerrainParamsChangeMap(t *testing.T) {
	p1 := testParams()
	p2 := testParams()
	p2.SeaLevel = 0.2

	w1, err := world.New(1, p1)
	if err != nil {
		t.Fatal(err)
	}
	w2, err := world.New(1, p2)
	if err != nil {
		t.Fatal(err)
	}
	ch1, err := worldgen.GenerateChunk(w1, world.ChunkCoord{X: 0, Y: 0})
	if err != nil {
		t.Fatal(err)
	}
	ch2, err := worldgen.GenerateChunk(w2, world.ChunkCoord{X: 0, Y: 0})
	if err != nil {
		t.Fatal(err)
	}
	if string(chunkBytes(t, ch1)) == string(chunkBytes(t, ch2)) {
		t.Fatal("changing sea level did not change the map")
	}
}
