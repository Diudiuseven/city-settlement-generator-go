package worldgen

import (
	"testing"
)

// TestRoadsOnlyOnPassable verifies every path tile is a passable cell
// and consecutive tiles are 8-connected with no corner cutting.
func TestRoadsOnlyOnPassable(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 2)
	view := newTerrainView(p, w.overlay)
	for _, e := range w.Roads().Edges {
		for i, pt := range e.Path {
			c := view.cellAt(pt.X, pt.Y)
			if !Passable(c.Biome) {
				t.Fatalf("edge %d-%d crosses impassable biome at (%d,%d)",
					e.A, e.B, pt.X, pt.Y)
			}
			if i > 0 {
				prev := e.Path[i-1]
				dx := absInt(pt.X - prev.X)
				dy := absInt(pt.Y - prev.Y)
				if dx > 1 || dy > 1 || (dx == 0 && dy == 0) {
					t.Fatalf("edge %d-%d not 8-connected", e.A, e.B)
				}
				if dx == 1 && dy == 1 {
					if !Passable(view.cellAt(pt.X, prev.Y).Biome) ||
						!Passable(view.cellAt(prev.X, pt.Y).Biome) {
						t.Fatalf("edge %d-%d cuts a corner at (%d,%d)",
							e.A, e.B, pt.X, pt.Y)
					}
				}
			}
		}
	}
}

// TestRoadGraphConnectsAllSettlements verifies requirement 4: every
// settlement is reachable from every other.
func TestRoadGraphConnectsAllSettlements(t *testing.T) {
	for _, seed := range []int64{1, 2, 7, 13, 99} {
		p := testParams()
		p.WorldSeed = seed
		w := rebuildWorld(t, p, 1)
		if !roadsConnected(w) {
			t.Fatalf("seed %d: road graph not connected", seed)
		}
	}
}

func roadsConnected(w *World) bool {
	ss := w.Settlements()
	if len(ss) <= 1 {
		return true
	}
	adj := map[uint64]map[uint64]bool{}
	for _, s := range ss {
		adj[s.ID] = map[uint64]bool{}
	}
	for _, e := range w.Roads().Edges {
		adj[e.A][e.B] = true
		adj[e.B][e.A] = true
	}
	seen := map[uint64]bool{ss[0].ID: true}
	st := []uint64{ss[0].ID}
	for len(st) > 0 {
		x := st[len(st)-1]
		st = st[:len(st)-1]
		for y := range adj[x] {
			if !seen[y] {
				seen[y] = true
				st = append(st, y)
			}
		}
	}
	return len(seen) == len(ss)
}

// TestRoadEndpointsAttach ensures each edge starts/ends at its
// declared settlement tiles and paths are canonical (a -> b, id a<b).
func TestRoadEndpointsAttach(t *testing.T) {
	p := testParams()
	w := rebuildWorld(t, p, 1)
	pos := map[uint64][2]int{}
	for _, s := range w.Settlements() {
		pos[s.ID] = [2]int{s.X, s.Y}
	}
	for _, e := range w.Roads().Edges {
		if e.A >= e.B {
			t.Fatalf("edge not canonical: %d >= %d", e.A, e.B)
		}
		first, last := e.Path[0], e.Path[len(e.Path)-1]
		pa, pb := pos[e.A], pos[e.B]
		if first != (RoadPoint{X: pa[0], Y: pa[1]}) ||
			last != (RoadPoint{X: pb[0], Y: pb[1]}) {
			t.Fatalf("edge %d-%d endpoints wrong: %v..%v want %v..%v",
				e.A, e.B, first, last, pa, pb)
		}
	}
}

// TestRoadGraphDeterministicAcrossSeedsAndConc is the determinism
// guarantee for the derived network.
func TestRoadGraphDeterministicAcrossSeedsAndConc(t *testing.T) {
	p := testParams()
	ref := roadSignature(rebuildWorld(t, p, 1))
	for _, c := range []int{2, 4, 9} {
		if got := roadSignature(rebuildWorld(t, p, c)); got != ref {
			t.Fatalf("road graph changed with concurrency %d", c)
		}
	}
}

func roadSignature(w *World) string {
	b := EncodeWorld(w)
	// Only the road section matters, but the full-world encoding is
	// itself canonical, so comparing bytes suffices.
	return string(b)
}

// TestNoRouteOnHostileTerrain exercises the A* failure path directly:
// if the goal cell is mountain/water, no path exists.
func TestNoRouteOnHostileTerrain(t *testing.T) {
	p := testParams()
	view := newTerrainView(p, newEditOverlay())
	planner := newRoadPlanner(p, view)
	// Find two settlements and confirm routing between them; then
	// confirm routing to a coordinate just inside the impassable
	// border sentinel returns nil.
	ss := planSettlements(p, view)
	if len(ss) >= 2 {
		path := planner.shortestPath(ss[0].X, ss[0].Y, ss[1].X, ss[1].Y)
		if path == nil {
			t.Fatal("expected route between two settlements")
		}
	}
	if path := planner.shortestPath(ss[0].X, ss[0].Y, -1, -1); path != nil {
		t.Fatal("route to out-of-range impassable tile should fail")
	}
}
