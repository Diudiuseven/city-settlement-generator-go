package worldgen

import (
	"context"
	"encoding/hex"
	"testing"
)

func testParams() Params {
	p := DefaultParams()
	p.Width = 128
	p.Height = 128
	p.ChunkSize = 64
	return p
}

func allTestCoords(p Params) []ChunkCoord {
	var out []ChunkCoord
	nx := p.Width / p.ChunkSize
	ny := p.Height / p.ChunkSize
	for cy := 0; cy < ny; cy++ {
		for cx := 0; cx < nx; cx++ {
			out = append(out, ChunkCoord{CX: cx, CY: cy})
		}
	}
	return out
}

// rebuildWorld builds a complete world with a given concurrency.
func rebuildWorld(t *testing.T, p Params, conc int) *World {
	t.Helper()
	w, err := NewWorld(p)
	if err != nil {
		t.Fatalf("NewWorld: %v", err)
	}
	if err := w.Rebuild(context.Background(), GenerateOptions{Concurrency: conc}); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	return w
}

// TestDeterminismSameSeedSameWorld is the root guarantee: same seed
// and params, separate processes' worth of independent worlds, must
// produce byte-identical maps.
func TestDeterminismSameSeedSameWorld(t *testing.T) {
	p := testParams()
	w1 := rebuildWorld(t, p, 1)
	w2 := rebuildWorld(t, p, 1)
	b1 := EncodeWorld(w1)
	b2 := EncodeWorld(w2)
	if string(b1) != string(b2) {
		t.Fatalf("independent rebuilds differ (%d vs %d bytes)", len(b1), len(b2))
	}
}

// TestDeterminismAcrossConcurrency proves the schedule/worker count
// cannot influence the result.
func TestDeterminismAcrossConcurrency(t *testing.T) {
	p := testParams()
	ref := EncodeWorld(rebuildWorld(t, p, 1))
	for _, c := range []int{2, 3, 4, 8, 16} {
		got := EncodeWorld(rebuildWorld(t, p, c))
		if string(got) != string(ref) {
			t.Fatalf("concurrency %d changed the map", c)
		}
	}
}

// TestDeterminismChunkBatching proves generating a chunk on its own
// gives identical cells to generating it together with neighbors.
func TestDeterminismChunkBatching(t *testing.T) {
	p := testParams()
	coords := allTestCoords(p)

	solo := make(map[ChunkCoord]*Chunk)
	for _, c := range coords {
		w, _ := NewWorld(p)
		if err := w.GenerateChunks(context.Background(), []ChunkCoord{c},
			GenerateOptions{Concurrency: 1}); err != nil {
			t.Fatal(err)
		}
		ch, ok := w.Chunk(c)
		if !ok {
			t.Fatal("chunk missing")
		}
		solo[c] = ch
	}

	wAll := rebuildWorld(t, p, 4)
	for c, a := range solo {
		b, ok := wAll.Chunk(c)
		if !ok {
			t.Fatalf("chunk %v missing from batch", c)
		}
		if !chunksEqual(a, b) {
			t.Fatalf("chunk %v differs solo vs batched", c)
		}
	}
}

func chunksEqual(a, b *Chunk) bool {
	if a.Size != b.Size || len(a.Cells) != len(b.Cells) {
		return false
	}
	for i := range a.Cells {
		if a.Cells[i] != b.Cells[i] {
			return false
		}
	}
	return true
}

// TestDifferentSeedsDiffer guards against accidentally ignoring seed.
func TestDifferentSeedsDiffer(t *testing.T) {
	p1, p2 := testParams(), testParams()
	p2.WorldSeed = 424242
	b1 := EncodeWorld(rebuildWorld(t, p1, 2))
	b2 := EncodeWorld(rebuildWorld(t, p2, 2))
	if string(b1) == string(b2) {
		t.Fatal("different seeds produced identical worlds")
	}
}

// TestTerrainDigestPinned locks the version-1 terrain digest forever.
// It must never change; old parameter combinations must remain
// reproducible.
func TestTerrainDigestPinned(t *testing.T) {
	p := testParams()
	d := p.TerrainDigest()
	got := hex.EncodeToString(d[:8])
	// Golden value captured for the frozen v1 digest serialization
	// layout. If this changes, the v1 reproducibility contract broke.
	const want = "f5c4b3fe026ea022"
	if got != want {
		t.Fatalf("terrain digest drifted: got %s want %s", got, want)
	}
}

// TestDerivedParamsExcludedFromTerrainDigest proves NameSalt etc. do
// not touch terrain identity.
func TestDerivedParamsExcludedFromTerrainDigest(t *testing.T) {
	p1 := testParams()
	p2 := p1
	p2.NameSalt = 999
	p2.MinSuitability = 1
	p2.RoadCostScale = 7
	if p1.TerrainDigest() != p2.TerrainDigest() {
		t.Fatal("terrain-independent params changed TerrainDigest")
	}
	if p1.Digest() == p2.Digest() {
		t.Fatal("full digest should reflect derived params")
	}
}

// TestNoiseDeterministicAndSeamless checks the noise primitive
// directly.
func TestNoiseDeterministicAndSeamless(t *testing.T) {
	n := newNoiseState(123, 40)
	a := n.valueNoise2D(1000, -250, 7)
	b := n.valueNoise2D(1000, -250, 7)
	if a != b || a > 65535 {
		t.Fatalf("noise unstable/out of range: %d %d", a, b)
	}
	// FBM must be continuous at integer positions (no NaN-style jumps;
	// here: adjacent cells never differ by more than a bounded amount).
	prev := n.fbmHeight(0, 0)
	for x := 1; x < 200; x++ {
		cur := n.fbmHeight(x, 0)
		if diff := int(cur) - int(prev); diff > 8000 || diff < -8000 {
			t.Fatalf("height discontinuity at x=%d: %d -> %d", x, prev, cur)
		}
		prev = cur
	}
}
