package worldgen

import (
	"testing"
)

// TestChunkSeamsVerticalHorizontal compares cells immediately on both
// sides of every internal chunk border. Since terrain is a pure point
// function, the two sides must be as continuous as any other adjacent
// pair in the interior of a chunk: no border-specific jump may exist.
func TestChunkSeamsVerticalHorizontal(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 1)

	const maxJump = 9000 // FBM Lipschitz bound on adjacent cells

	// A reference over all adjacent pairs inside chunks; borders must
	// not exhibit any larger jump than the interior maximum.
	interiorMax := 0
	borderMax := 0

	for cy := 0; cy < p.Height/p.ChunkSize; cy++ {
		for cx := 0; cx < p.Width/p.ChunkSize; cx++ {
			ch := mustChunk(t, w, ChunkCoord{cx, cy})
			for ly := 0; ly < ch.Size; ly++ {
				for lx := 0; lx < ch.Size; lx++ {
					c := ch.Cells[ly*ch.Size+lx]
					if lx+1 < ch.Size {
						n := ch.Cells[ly*ch.Size+lx+1]
						interiorMax = maxInt(interiorMax, jump(c, n))
					}
					if ly+1 < ch.Size {
						n := ch.Cells[(ly+1)*ch.Size+lx]
						interiorMax = maxInt(interiorMax, jump(c, n))
					}
				}
			}
		}
	}

	// Compare across vertical and horizontal borders.
	for cy := 0; cy < p.Height/p.ChunkSize; cy++ {
		for cx := 0; cx < p.Width/p.ChunkSize; cx++ {
			ch := mustChunk(t, w, ChunkCoord{cx, cy})
			if cx+1 < p.Width/p.ChunkSize {
				right := mustChunk(t, w, ChunkCoord{cx + 1, cy})
				for ly := 0; ly < ch.Size; ly++ {
					a := ch.Cells[ly*ch.Size+ch.Size-1]
					b := right.Cells[ly*ch.Size]
					borderMax = maxInt(borderMax, jump(a, b))
					if j := jump(a, b); j > maxJump {
						t.Fatalf("vertical seam jump %d at chunk(%d,%d) row %d", j, cx, cy, ly)
					}
				}
			}
			if cy+1 < p.Height/p.ChunkSize {
				below := mustChunk(t, w, ChunkCoord{cx, cy + 1})
				for lx := 0; lx < ch.Size; lx++ {
					a := ch.Cells[(ch.Size-1)*ch.Size+lx]
					b := below.Cells[lx]
					borderMax = maxInt(borderMax, jump(a, b))
					if j := jump(a, b); j > maxJump {
						t.Fatalf("horizontal seam jump %d at chunk(%d,%d) col %d", j, cx, cy, lx)
					}
				}
			}
		}
	}
	// The strongest border jump must not exceed the strongest interior
	// jump (i.e. borders introduce nothing special).
	if borderMax > interiorMax {
		t.Fatalf("border jumps (%d) exceed interior jumps (%d)", borderMax, interiorMax)
	}
}

// TestChunkBorderBiomeSymmetry ensures shared classifications: biome
// is a pure function of (height, moisture), so identical height
// implies identical biome even across separately generated chunks.
func TestChunkBorderBiomeSymmetry(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 4)
	view := newTerrainView(p, w.overlay)
	for cy := 0; cy < p.Height/p.ChunkSize; cy++ {
		for cx := 0; cx < p.Width/p.ChunkSize; cx++ {
			ch := mustChunk(t, w, ChunkCoord{cx, cy})
			for ly := 0; ly < ch.Size; ly++ {
				for lx := 0; lx < ch.Size; lx++ {
					got := ch.Cells[ly*ch.Size+lx].Biome
					want := view.cellAt(cx*p.ChunkSize+lx, cy*p.ChunkSize+ly).Biome
					if got != want {
						t.Fatalf("biome mismatch at (%d,%d): chunk=%d point=%d",
							cx*p.ChunkSize+lx, cy*p.ChunkSize+ly, got, want)
					}
				}
			}
		}
	}
}

// TestSoloChunkEqualsBatchedChunk covers requirement 2's strongest
// statement: rendering a chunk alone equals rendering it with
// neighbors (already covered in determinism tests, re-asserted here
// with raw rendering against the batch world for clarity).
func TestSoloChunkEqualsBatchedChunk(t *testing.T) {
	p := testParams()
	for _, target := range []ChunkCoord{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		solo := renderChunk(p, newEditOverlay(), target)
		w := rebuildWorld(t, p, 2)
		batched := mustChunk(t, w, target)
		if !chunksEqual(solo, batched) {
			t.Fatalf("chunk %v differs solo vs batched", target)
		}
	}
}

func mustChunk(t *testing.T, w *World, c ChunkCoord) *Chunk {
	t.Helper()
	ch, ok := w.Chunk(c)
	if !ok {
		t.Fatalf("missing chunk %v", c)
	}
	return ch
}

func jump(a, b Cell) int {
	h := int(a.Height) - int(b.Height)
	if h < 0 {
		h = -h
	}
	return h
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
