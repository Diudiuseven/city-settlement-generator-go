package worldgen

import (
	"container/heap"
	"errors"
	"sort"
)

// ErrNoRoute means two points that must be connected are mutually
// unreachable over passable terrain (for example because an edit
// walled a region off).
var ErrNoRoute = errors.New("worldgen: no road route between settlements")

// Traversal costs. They are deterministic integers; RoadCostScale
// (a derived-only parameter) linearly scales them without changing
// which routes exist.
const (
	costGrassland uint64 = 10
	costForest    uint64 = 14
	costDesert    uint64 = 20
	// Diagonal steps multiply the orthogonal cost by this/10.
	diagFactorNum = 14
	diagFactorDen = 10
)

func biomeCost(b Biome) uint64 {
	switch b {
	case BiomeGrassland:
		return costGrassland
	case BiomeForest:
		return costForest
	case BiomeDesert:
		return costDesert
	default:
		return 0 // impassable
	}
}

// roadPlanner holds a dense snapshot of passability and per-cell
// traversal cost. Routing many settlement pairs from one snapshot is
// far cheaper than re-evaluating terrain noise inside every A* step,
// and the snapshot is a deterministic derivation of the terrain, so
// results are unchanged.
type roadPlanner struct {
	p     Params
	w, h  int
	pass  []bool   // w*h
	cost  []uint16 // orthogonal step cost per destination cell
	scale uint64
	// scratch search state (reset per query via stamps)
	g     []uint64
	stamp []uint32
	epoch uint32
	came  []int32
	pq    aStarPQ
}

func newRoadPlanner(p Params, view terrainView) *roadPlanner {
	n := p.Width * p.Height
	pl := &roadPlanner{
		p:     p,
		w:     p.Width,
		h:     p.Height,
		pass:  make([]bool, n),
		cost:  make([]uint16, n),
		g:     make([]uint64, n),
		stamp: make([]uint32, n),
		came:  make([]int32, n),
	}
	scale := p.RoadCostScale
	if scale == 0 {
		scale = 1
	}
	pl.scale = scale
	for y := 0; y < p.Height; y++ {
		for x := 0; x < p.Width; x++ {
			c := view.cellAt(x, y)
			idx := y*p.Width + x
			if Passable(c.Biome) {
				pl.pass[idx] = true
				pl.cost[idx] = uint16(biomeCost(c.Biome))
			}
		}
	}
	heap.Init(&pl.pq)
	return pl
}

// aStarNode is the per-cell open-set entry.
type aStarNode struct {
	key   uint64
	f, g  uint64
	index int
}

type aStarPQ []*aStarNode

func (q aStarPQ) Len() int { return len(q) }
func (q aStarPQ) Less(i, j int) bool {
	if q[i].f != q[j].f {
		return q[i].f < q[j].f
	}
	return q[i].key < q[j].key // deterministic tie break
}
func (q aStarPQ) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}
func (q *aStarPQ) Push(x any) {
	n := x.(*aStarNode)
	n.index = len(*q)
	*q = append(*q, n)
}
func (q *aStarPQ) Pop() any {
	old := *q
	n := old[len(old)-1]
	*q = old[:len(old)-1]
	return n
}

// shortestPath runs deterministic A* on the dense snapshot. Diagonal
// corner cutting is forbidden. Returns nil when no route exists.
func (pl *roadPlanner) shortestPath(x1, y1, x2, y2 int) []RoadPoint {
	w := pl.w
	start := y1*w + x1
	goal := y2*w + x2
	if x1 < 0 || y1 < 0 || x2 < 0 || y2 < 0 ||
		x1 >= pl.w || y1 >= pl.h || x2 >= pl.w || y2 >= pl.h {
		return nil
	}
	if x1 == x2 && y1 == y2 {
		return []RoadPoint{{X: x1, Y: y1}}
	}
	if !pl.pass[start] || !pl.pass[goal] {
		return nil
	}
	pl.epoch++
	ep := pl.epoch
	heur := func(idx int) uint64 {
		cx, cy := idx%w, idx/w
		dx := absInt(cx - x2)
		dy := absInt(cy - y2)
		lo, hi := dx, dy
		if lo > hi {
			lo, hi = hi, lo
		}
		minStep := costGrassland
		return uint64(hi-lo)*minStep +
			uint64(lo)*minStep*diagFactorNum/diagFactorDen
	}

	pl.pq = pl.pq[:0]
	pl.stamp[start] = ep
	pl.g[start] = 0
	pl.came[start] = -1
	heap.Push(&pl.pq, &aStarNode{key: uint64(start), f: heur(start), g: 0})

	for pl.pq.Len() > 0 {
		cur := heap.Pop(&pl.pq).(*aStarNode)
		if cur.key == uint64(goal) {
			return pl.reconstruct(goal)
		}
		cx, cy := cur.key%uint64(w), cur.key/uint64(w)
		ci := int(cur.key)
		// Outdated duplicate entries are skipped by stamp/g compare.
		if pl.stamp[ci] != ep || pl.g[ci] != cur.g {
			continue
		}
		for dy := -1; dy <= 1; dy++ {
			ny := int(cy) + dy
			if ny < 0 || ny >= pl.h {
				continue
			}
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				nx := int(cx) + dx
				if nx < 0 || nx >= w {
					continue
				}
				ni := ny*w + nx
				if !pl.pass[ni] {
					continue
				}
				if dx != 0 && dy != 0 {
					if !pl.pass[ci+dx] || !pl.pass[ci+dy*w] {
						continue // no corner cutting
					}
				}
				step := uint64(pl.cost[ni])
				if dx != 0 && dy != 0 {
					step = step * diagFactorNum / diagFactorDen
				}
				step *= pl.scale
				ng := cur.g + step
				if pl.stamp[ni] == ep && ng >= pl.g[ni] {
					continue
				}
				pl.stamp[ni] = ep
				pl.g[ni] = ng
				pl.came[ni] = int32(ci)
				heap.Push(&pl.pq, &aStarNode{
					key: uint64(ni), g: ng, f: ng + heur(ni),
				})
			}
		}
	}
	return nil
}

func (pl *roadPlanner) reconstruct(goal int) []RoadPoint {
	var rev []RoadPoint
	k := goal
	for k >= 0 {
		rev = append(rev, RoadPoint{X: k % pl.w, Y: k / pl.w})
		k = int(pl.came[k])
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// pathCost computes the exact traversal cost of a path from the
// snapshot, so an edge weight is identical regardless of which
// endpoint the search started from.
func (pl *roadPlanner) pathCost(path []RoadPoint) uint64 {
	var total uint64
	for i := 1; i < len(path); i++ {
		ni := path[i].Y*pl.w + path[i].X
		step := uint64(pl.cost[ni])
		if step == 0 {
			return 0
		}
		dx := absInt(path[i].X - path[i-1].X)
		dy := absInt(path[i].Y - path[i-1].Y)
		if dx != 0 && dy != 0 {
			step = step * diagFactorNum / diagFactorDen
		}
		total += step * pl.scale
	}
	return total
}

// roadCandidate is one weighted possible edge.
type roadCandidate struct {
	a, b uint64
	cost uint64
	path []RoadPoint
}

// buildRoadGraph connects every settlement with a deterministic
// minimum spanning tree:
//
//  1. Shortest routes are computed to each settlement's nearest
//     lattice neighbors (bounded A* calls).
//  2. If those edges leave multiple components, routes between the
//     closest pair of remaining components are added. The structural
//     terrain corridors guarantee these routes exist for unedited
//     terrain.
//  3. Kruskal selects the canonical MST (ordered by cost, then IDs).
//
// If any required pair is unreachable (only possible after hostile
// edits), ErrNoRoute is returned and the caller rejects the edit batch.
func buildRoadGraph(p Params, view terrainView, ss []Settlement) (*RoadGraph, error) {
	if len(ss) == 0 {
		return &RoadGraph{}, nil
	}
	planner := newRoadPlanner(p, view)

	byID := make(map[uint64]Settlement, len(ss))
	for _, s := range ss {
		byID[s.ID] = s
	}
	ids := make([]uint64, 0, len(ss))
	for _, s := range ss {
		ids = append(ids, s.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	// Neighbor radius in lattice cells for the bounded candidate set.
	const neighborRadius = 2
	s := p.SettlementSpacing
	radiusPx := neighborRadius*s + p.SettlementJitter + 1

	var cands []roadCandidate
	addRoute := func(a, b uint64) {
		sa, sb := byID[a], byID[b]
		path := planner.shortestPath(sa.X, sa.Y, sb.X, sb.Y)
		if path == nil {
			return
		}
		cands = append(cands, roadCandidate{a: a, b: b, cost: planner.pathCost(path), path: path})
	}
	for i, ida := range ids {
		sa := byID[ida]
		for j := i + 1; j < len(ids); j++ {
			idb := ids[j]
			sb := byID[idb]
			if absInt(sa.X-sb.X) <= radiusPx && absInt(sa.Y-sb.Y) <= radiusPx {
				addRoute(ida, idb)
			}
		}
	}

	parent := make([]uint64, 0)
	idIndex := map[uint64]int{}
	for i, id := range ids {
		idIndex[id] = i
		parent = append(parent, uint64(i))
	}
	var find func(int) int
	find = func(x int) int {
		for int(parent[x]) != x {
			parent[x] = parent[parent[x]]
			x = int(parent[x])
		}
		return x
	}

	sortCandidates := func() {
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].cost != cands[j].cost {
				return cands[i].cost < cands[j].cost
			}
			if cands[i].a != cands[j].a {
				return cands[i].a < cands[j].a
			}
			return cands[i].b < cands[j].b
		})
	}

	var chosen []roadCandidate
	for {
		sortCandidates()
		chosen = chosen[:0]
		for i := range parent {
			parent[i] = uint64(i)
		}
		for _, c := range cands {
			ra, rb := find(idIndex[c.a]), find(idIndex[c.b])
			if ra == rb {
				continue
			}
			parent[ra] = uint64(rb)
			chosen = append(chosen, c)
			if len(chosen) == len(ids)-1 {
				break
			}
		}
		root0 := find(0)
		connected := true
		for i := 1; i < len(ids); i++ {
			if find(i) != root0 {
				connected = false
				break
			}
		}
		if connected {
			break
		}
		added, err := connectComponents(p, planner, byID, ids, idIndex, find, &cands)
		if err != nil {
			return nil, err
		}
		if !added {
			return nil, ErrNoRoute
		}
	}

	sort.Slice(chosen, func(i, j int) bool {
		if chosen[i].a != chosen[j].a {
			return chosen[i].a < chosen[j].a
		}
		return chosen[i].b < chosen[j].b
	})
	g := &RoadGraph{Edges: make([]RoadEdge, 0, len(chosen))}
	for _, c := range chosen {
		a := c.a
		path := c.path
		sa := byID[a]
		if path[0].X != sa.X || path[0].Y != sa.Y {
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
		}
		g.Edges = append(g.Edges, RoadEdge{A: c.a, B: c.b, Cost: c.cost, Path: path})
	}
	return g, nil
}

// connectComponents finds the shortest cross-component route and
// appends it. Returns added=false if no cross-component route exists.
func connectComponents(
	p Params,
	planner *roadPlanner,
	byID map[uint64]Settlement,
	ids []uint64,
	idIndex map[uint64]int,
	find func(int) int,
	cands *[]roadCandidate,
) (bool, error) {
	type pair struct {
		cost uint64
		a, b uint64
		path []RoadPoint
	}
	best := pair{}
	found := false
	for i, ida := range ids {
		sa := byID[ida]
		ra := find(i)
		for j := i + 1; j < len(ids); j++ {
			if ra == find(j) {
				continue
			}
			idb := ids[j]
			sb := byID[idb]
			path := planner.shortestPath(sa.X, sa.Y, sb.X, sb.Y)
			if path == nil {
				continue
			}
			c := planner.pathCost(path)
			if !found || c < best.cost ||
				(c == best.cost && (ida < best.a || (ida == best.a && idb < best.b))) {
				found = true
				best = pair{cost: c, a: ida, b: idb, path: path}
			}
		}
	}
	if !found {
		return false, ErrNoRoute
	}
	*cands = append(*cands, roadCandidate{a: best.a, b: best.b, cost: best.cost, path: best.path})
	return true, nil
}
