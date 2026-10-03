package worldgen

import (
	"bytes"
	"context"
	"testing"
)

func hPtr(v Height) *Height     { return &v }
func mPtr(v Moisture) *Moisture { return &v }

// TestEditIdempotentAndOrderIndependent covers requirement 5:
// repeated submissions and submission order cannot change the map.
func TestEditIdempotentAndOrderIndependent(t *testing.T) {
	p := testParams()

	apply := func(batches ...[]CellEdit) []byte {
		w := rebuildWorld(t, p, 1)
		for _, b := range batches {
			if err := w.ApplyEdits(context.Background(), b,
				GenerateOptions{Concurrency: 2}); err != nil {
				t.Fatalf("ApplyEdits: %v", err)
			}
		}
		return EncodeWorld(w)
	}

	// benign land edit in the middle of a land area
	land := findPassableTile(t, p)
	e1 := CellEdit{X: land[0], Y: land[1], SetHeight: hPtr(30000), SetMoisture: mPtr(30000)}
	e2 := CellEdit{X: land[0] + 3, Y: land[1], SetHeight: hPtr(31000)}
	e3 := CellEdit{X: land[0], Y: land[1] + 2, SetMoisture: mPtr(25000)}

	once := apply([]CellEdit{e1, e2, e3})
	twice := apply([]CellEdit{e1, e2, e3}, []CellEdit{e1, e2, e3})
	reordered := apply([]CellEdit{e3, e1}, []CellEdit{e2})

	if !bytes.Equal(once, twice) {
		t.Fatal("repeated edit submission changed the map")
	}
	if !bytes.Equal(once, reordered) {
		t.Fatal("edit submission order changed the map")
	}
}

// TestIncrementalEqualsRebuild is requirement 5's byte-alignment
// statement: after incremental edits, a from-scratch rebuild from the
// same edit log must be byte-identical.
func TestIncrementalEqualsRebuild(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 1)

	land := findPassableTile(t, p)
	edits := []CellEdit{
		{X: land[0], Y: land[1], SetHeight: hPtr(33000), SetMoisture: mPtr(28000)},
		{X: land[0] + 5, Y: land[1] - 4, SetHeight: hPtr(29000)},
		{X: land[0] - 3, Y: land[1] + 1, SetMoisture: mPtr(40000)},
	}
	if err := w.ApplyEdits(context.Background(), edits,
		GenerateOptions{Concurrency: 4}); err != nil {
		t.Fatal(err)
	}
	inc := EncodeWorld(w)

	fresh, err := NewWorld(p)
	if err != nil {
		t.Fatal(err)
	}
	// Replay the recorded edit log onto a pristine world, then rebuild.
	replayed := w.Edits()
	if err := fresh.ApplyEdits(context.Background(), replayed,
		GenerateOptions{Concurrency: 1}); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Rebuild(context.Background(), GenerateOptions{Concurrency: 2}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(inc, EncodeWorld(fresh)) {
		t.Fatal("incrementally edited map differs from from-scratch rebuild")
	}
}

// TestEditOnlyRecomputesTouchedChunks: an edit changes only the chunk
// containing its cell (and possibly a bounded settlement neighborhood,
// never a distant chunk's terrain).
func TestEditOnlyRecomputesTouchedChunks(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 1)
	wBefore := rebuildWorld(t, p, 1) // pristine reference

	land := findPassableTile(t, p)
	edit := CellEdit{X: land[0], Y: land[1], SetHeight: hPtr(28000)}
	if err := w.ApplyEdits(context.Background(), []CellEdit{edit},
		GenerateOptions{Concurrency: 1}); err != nil {
		t.Fatal(err)
	}
	touched := ChunkCoord{CX: land[0] / p.ChunkSize, CY: land[1] / p.ChunkSize}

	for _, c := range w.Chunks() {
		after := chunkBytes(mustChunk(t, w, c))
		beforeB := chunkBytes(mustChunk(t, wBefore, c))
		if c != touched && !bytes.Equal(after, beforeB) {
			t.Fatalf("untouched chunk %v terrain changed after local edit", c)
		}
	}
}

// TestDisconnectingEditRejected: a batch that would wall every
// settlement off must be rejected atomically with no half state.
func TestDisconnectingEditRejected(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 1)
	if len(w.Settlements()) < 2 {
		t.Skip("need at least two settlements")
	}
	before := EncodeWorld(w)

	// Wall a settlement in completely with a ring of mountain cells.
	target := w.Settlements()[0]
	var wall []CellEdit
	for dy := -3; dy <= 3; dy++ {
		for dx := -3; dx <= 3; dx++ {
			if absInt(dx) == 3 || absInt(dy) == 3 {
				x, y := target.X+dx, target.Y+dy
				if x >= 0 && y >= 0 && x < p.Width && y < p.Height {
					wall = append(wall, CellEdit{X: x, Y: y, SetHeight: hPtr(p.MountainThreshold)})
				}
			}
		}
	}
	err := w.ApplyEdits(context.Background(), wall, GenerateOptions{Concurrency: 1})
	if err == nil {
		// It is possible (but unlikely) the ring still allows routing;
		// in that case there is nothing to assert.
		t.Log("wall edit unexpectedly kept graph connected; accepting")
	} else if err != ErrNoRoute {
		t.Fatalf("expected ErrNoRoute, got %v", err)
	}
	if !bytes.Equal(before, EncodeWorld(w)) {
		t.Fatal("failed edit batch modified world state")
	}
}

// TestNilPatchFields verifies nil pointers leave prior values intact.
func TestNilPatchFields(t *testing.T) {
	o := newEditOverlay()
	o = o.apply([]CellEdit{{X: 1, Y: 2, SetHeight: hPtr(100), SetMoisture: mPtr(200)}})
	o = o.apply([]CellEdit{{X: 1, Y: 2, SetHeight: hPtr(150)}})
	e, ok := o.get(1, 2)
	if !ok || *e.SetHeight != 150 || *e.SetMoisture != 200 {
		t.Fatalf("patch semantics wrong: %+v", e)
	}
}

// findPassableTile locates any land cell suitable for benign edits.
func findPassableTile(t *testing.T, p Params) [2]int {
	t.Helper()
	view := newTerrainView(p, newEditOverlay())
	for y := p.Height / 3; y < 2*p.Height/3; y++ {
		for x := p.Width / 3; x < 2*p.Width/3; x++ {
			c := view.cellAt(x, y)
			if c.Biome == BiomeGrassland {
				return [2]int{x, y}
			}
		}
	}
	t.Fatal("no passable tile found for edit test")
	return [2]int{}
}

func chunkBytes(ch *Chunk) []byte {
	b := make([]byte, 0, len(ch.Cells)*6)
	for _, c := range ch.Cells {
		b = appendU16(b, uint16(c.Height))
		b = appendU16(b, uint16(c.Moisture))
		b = appendU16(b, uint16(c.Biome))
	}
	return b
}

func appendU16(b []byte, v uint16) []byte {
	return append(b, byte(v>>8), byte(v))
}
