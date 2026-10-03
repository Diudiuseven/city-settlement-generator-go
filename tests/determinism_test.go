package tests

import (
	"context"
	"testing"

	"github.com/Diudiuseven/city-settlement-generator-go/pipeline"
	"github.com/Diudiuseven/city-settlement-generator-go/world"
)

func testParams() world.Params {
	p := world.DefaultParams()
	p.ChunkSize = 32
	return p
}

func testCoords() []world.ChunkCoord {
	return []world.ChunkCoord{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0},
		{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 2, Y: 1},
		{X: 0, Y: 2}, {X: 1, Y: 2}, {X: 2, Y: 2},
	}
}

// generateWorld builds a world and generates the given chunks with the given
// worker count.
func generateWorld(t *testing.T, seed int64, coords []world.ChunkCoord, workers int) *world.World {
	t.Helper()
	w, err := world.New(seed, testParams())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.Generate(context.Background(), w, coords, pipeline.Options{Workers: workers}); err != nil {
		t.Fatal(err)
	}
	return w
}

func chunkBytes(t *testing.T, ch *world.Chunk) []byte {
	t.Helper()
	b, err := ch.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestDeterminism: the same seed + parameters produce byte-identical chunks
// across independent worlds.
func TestDeterminism(t *testing.T) {
	coords := testCoords()
	w1 := generateWorld(t, 1234, coords, 4)
	w2 := generateWorld(t, 1234, coords, 4)
	for _, c := range coords {
		b1 := chunkBytes(t, w1.GetChunk(c))
		b2 := chunkBytes(t, w2.GetChunk(c))
		if string(b1) != string(b2) {
			t.Fatalf("chunk %v differs between identical worlds", c)
		}
		h1, _ := w1.GetChunk(c).StableHash()
		h2, _ := w2.GetChunk(c).StableHash()
		if h1 != h2 {
			t.Fatalf("chunk %v hash differs: %x vs %x", c, h1, h2)
		}
	}
}

// TestDifferentSeedsDiffer: a different seed must change the world.
func TestDifferentSeedsDiffer(t *testing.T) {
	coords := testCoords()
	w1 := generateWorld(t, 1, coords, 4)
	w2 := generateWorld(t, 2, coords, 4)
	different := false
	for _, c := range coords {
		if string(chunkBytes(t, w1.GetChunk(c))) != string(chunkBytes(t, w2.GetChunk(c))) {
			different = true
		}
	}
	if !different {
		t.Fatal("different seeds produced identical worlds")
	}
}

// TestEncodingRoundTrip: marshal -> unmarshal yields identical bytes.
func TestEncodingRoundTrip(t *testing.T) {
	w := generateWorld(t, 99, testCoords(), 4)
	for _, c := range testCoords() {
		ch := w.GetChunk(c)
		b, err := ch.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		back := &world.Chunk{}
		if err := back.UnmarshalBinary(b); err != nil {
			t.Fatal(err)
		}
		b2, err := back.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != string(b2) {
			t.Fatalf("chunk %v not stable under round-trip", c)
		}
	}
}

// TestGoldenHashes pins the exact bytes produced by a fixed seed and
// parameter set. If this test fails, the v1 algorithm has changed and old
// maps are no longer reproducible.
func TestGoldenHashes(t *testing.T) {
	w := generateWorld(t, 20261003, []world.ChunkCoord{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1}}, 4)
	want := map[world.ChunkCoord]uint64{
		{X: 0, Y: 0}: 0x126f2b8059c97d8c,
		{X: 1, Y: 0}: 0x9803cc48ec985226,
		{X: 0, Y: 1}: 0xc53fa68e50378052,
		{X: 1, Y: 1}: 0x74c839ef1e1deec5,
	}
	for c, h := range want {
		got, err := w.GetChunk(c).StableHash()
		if err != nil {
			t.Fatal(err)
		}
		if got != h {
			t.Fatalf("chunk %v hash drift: got %016x want %016x (algorithm version changed?)", c, got, h)
		}
	}
}
