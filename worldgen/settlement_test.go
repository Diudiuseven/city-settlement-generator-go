package worldgen

import (
	"context"
	"testing"
)

// TestSettlementMinimumSpacing checks every pair of sites is at least
// MinSettlementDist2 apart and no two sites occupy the same tile.
func TestSettlementMinimumSpacing(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 2)
	ss := w.Settlements()
	if len(ss) < 2 {
		t.Skip("world produced too few settlements for this seed")
	}
	occ := map[[2]int]bool{}
	for i, a := range ss {
		if occ[[2]int{a.X, a.Y}] {
			t.Fatalf("duplicate settlement tile (%d,%d)", a.X, a.Y)
		}
		occ[[2]int{a.X, a.Y}] = true
		for j := i + 1; j < len(ss); j++ {
			b := ss[j]
			dx := a.X - b.X
			dy := a.Y - b.Y
			if d2 := dx*dx + dy*dy; d2 < p.MinSettlementDist2 {
				t.Fatalf("settlements %d and %d only %d cells apart (min %d)",
					a.ID, b.ID, d2, p.MinSettlementDist2)
			}
		}
	}
}

// TestSettlementSuitabilityConstraints verifies every accepted site is
// on passable terrain and scored at/above the threshold.
func TestSettlementSuitabilityConstraints(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 1)
	view := newTerrainView(p, w.overlay)
	for _, s := range w.Settlements() {
		c := view.cellAt(s.X, s.Y)
		if !Passable(c.Biome) {
			t.Fatalf("settlement %d on impassable biome %d", s.ID, c.Biome)
		}
		if score := settlementScore(view, p, s.X, s.Y); score < p.MinSuitability {
			t.Fatalf("settlement %d below suitability threshold: %d", s.ID, score)
		}
		if s.Suitability != settlementScore(view, p, s.X, s.Y) {
			t.Fatalf("settlement %d stored score inconsistent", s.ID)
		}
	}
}

// TestSettlementIDsStableAndOrdered ensures the site list is canonical.
func TestSettlementIDsStableAndOrdered(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 3)
	ss := w.Settlements()
	for i := 1; i < len(ss); i++ {
		if ss[i-1].ID >= ss[i].ID {
			t.Fatalf("settlement list not ID-ordered at %d", i)
		}
	}
}

// TestSettlementSeedStability is requirement 3's reproducibility:
// same seed/params -> identical sites, even across independent worlds.
func TestSettlementSeedStability(t *testing.T) {
	p := testParams()
	a := rebuildWorld(t, p, 1).Settlements()
	b := rebuildWorld(t, p, 8).Settlements()
	if len(a) != len(b) {
		t.Fatalf("site count differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("site %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
}

// TestJitterGuarantee validates the structural spacing argument:
// 2*jitter+2 <= spacing means any two lattice cells stay far enough.
func TestJitterGuarantee(t *testing.T) {
	p := testParams()
	view := newTerrainView(p, newEditOverlay())
	ss := planSettlements(p, view)
	minD2 := 1 << 30
	for i := range ss {
		for j := i + 1; j < len(ss); j++ {
			dx := ss[i].X - ss[j].X
			dy := ss[i].Y - ss[j].Y
			if d2 := dx*dx + dy*dy; d2 < minD2 {
				minD2 = d2
			}
		}
	}
	if len(ss) > 1 && minD2 < p.MinSettlementDist2 {
		t.Fatalf("lattice guarantee violated: %d < %d", minD2, p.MinSettlementDist2)
	}
}

// TestSettlementThresholdFilters proves the suitability parameter
// actually constrains placement.
func TestSettlementThresholdFilters(t *testing.T) {
	p := testParams()
	base := len(rebuildWorld(t, p, 1).Settlements())
	p2 := p
	p2.MinSuitability = 65000
	w2, err := NewWorld(p2)
	if err != nil {
		t.Fatal(err)
	}
	if err := w2.Rebuild(context.Background(), GenerateOptions{Concurrency: 1}); err != nil {
		t.Fatal(err)
	}
	if n := len(w2.Settlements()); n >= base {
		t.Fatalf("max threshold should filter sites, got %d >= %d", n, base)
	}
}
