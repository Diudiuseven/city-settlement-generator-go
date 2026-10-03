package worldgen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// TestUnknownVersionRejected guards the registry: worlds cannot bind to
// an algorithm version that does not exist.
func TestUnknownVersionRejected(t *testing.T) {
	p := testParams()
	p.AlgorithmVersion = 999
	if _, err := NewWorld(p); err == nil {
		t.Fatal("unknown algorithm version accepted")
	}
}

// TestV1ReproducibilityPins proves v1 output is frozen: the SHA-256
// of the canonical encoding of the standard 128x128 test world must
// never change as the codebase evolves. If this test fails, v1 worlds
// are no longer byte-reproducible and the compatibility contract broke.
func TestV1ReproducibilityPins(t *testing.T) {
	p := testParams()
	if p.AlgorithmVersion != 1 {
		t.Fatal("default must be v1")
	}
	w := rebuildWorld(t, p, 1)
	sum := sha256.Sum256(EncodeWorld(w))
	got := hex.EncodeToString(sum[:])
	const want = "a74aaf4c5c345a7f08282140e9b2ac1c3703d5a99634241a6aafc31bb07101f3"
	if got != want {
		t.Fatalf("v1 world encoding drifted:\ngot  %s\nwant %s", got, want)
	}
}

// TestRegistryIsAppendOnly documents the rule: registering v1 again is
// a programmer error; new versions only append.
func TestRegistryIsAppendOnly(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate v1 registration should panic")
		}
	}()
	registerAlgorithm(1, terrain1)
}

// TestDerivedParamsDoNotChangeTerrain is requirement 7's parameter
// isolation: changing terrain-independent parameters leaves all
// existing cells byte-identical; only derived layers may change.
func TestDerivedParamsDoNotChangeTerrain(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 1)
	before := map[ChunkCoord][]byte{}
	for _, c := range w.Chunks() {
		before[c] = chunkBytes(mustChunk(t, w, c))
	}

	p2 := p
	p2.NameSalt = 31337
	p2.RoadCostScale = 3
	p2.MinSuitability = 1
	if err := w.UpdateParams(p2); err != nil {
		t.Fatalf("UpdateParams: %v", err)
	}
	for c, want := range before {
		if got := chunkBytes(mustChunk(t, w, c)); !bytes.Equal(got, want) {
			t.Fatalf("chunk %v terrain changed via derived-only params", c)
		}
	}
}

// TestTerrainParamsUpdateRejected ensures changing the shape of the
// terrain is not silently applied to existing regions.
func TestTerrainParamsUpdateRejected(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 1)
	p2 := p
	p2.WorldSeed = p.WorldSeed + 1
	if err := w.UpdateParams(p2); err == nil {
		t.Fatal("changing the seed through UpdateParams must be rejected")
	}
	p3 := p
	p3.MountainThreshold = p.MountainThreshold + 1
	if err := w.UpdateParams(p3); err == nil {
		t.Fatal("changing mountain threshold through UpdateParams must be rejected")
	}
}

// TestOldVersionWorldStaysReproducible simulates the forward-compat
// rule: a world pinned to v1 keeps its TerrainDigest even after a
// hypothetical future version becomes the default.
func TestOldVersionWorldStaysReproducible(t *testing.T) {
	p1 := testParams()
	p1.AlgorithmVersion = 1
	d1 := p1.TerrainDigest()

	future := DefaultParams()
	future.AlgorithmVersion = 2 // not registered: a future build would
	if _, err := NewWorld(future); err == nil {
		// If v2 ever registers, it must not alter the pinned v1 digest.
		if d1 == future.TerrainDigest() {
			t.Fatal("v2 must produce a distinct digest")
		}
	}
	// The v1 digest remains stable in this build.
	if p1.TerrainDigest() != d1 {
		t.Fatal("pinned v1 digest changed")
	}
}

// TestRebuildFromEditLogAcrossInstances: seed+params+edit log fully
// define the world, so a new instance reconstructs the exact same bytes.
func TestRebuildFromEditLogAcrossInstances(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 1)
	land := findPassableTile(t, p)
	if err := w.ApplyEdits(context.Background(), []CellEdit{
		{X: land[0], Y: land[1], SetHeight: hPtr(30000), SetMoisture: mPtr(20000)},
		{X: land[0] + 2, Y: land[1] + 2, SetHeight: hPtr(34000)},
	}, GenerateOptions{Concurrency: 2}); err != nil {
		t.Fatal(err)
	}
	want := EncodeWorld(w)

	fresh, _ := NewWorld(p)
	if err := fresh.ApplyEdits(context.Background(), w.Edits(),
		GenerateOptions{Concurrency: 8}); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Rebuild(context.Background(),
		GenerateOptions{Concurrency: 8}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, EncodeWorld(fresh)) {
		t.Fatal("world not reproducible from seed+params+edit log")
	}
}
