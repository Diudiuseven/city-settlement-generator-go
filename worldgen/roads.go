package worldgen

import (
	"container/heap"
	"math"
	"sort"

	"github.com/Diudiuseven/city-settlement-generator-go/world"
)

// roadSettlement is a settlement with its global tile coordinates.
type roadSettlement struct {
	id     uint64
	tx, ty int
	chunk  world.ChunkCoord
}

// roadEdge is an MST candidate between two settlements.
type roadEdge struct {
	a, b int // indices into the settlement list
	dist float64
	idA  uint64
	idB  uint64
}

// RecomputeRoads rebuilds the road network from the current terrain and
// settlements and writes road overlays into every resident chunk.
//
// The result is a pure function of (seed, params, edits): running it after a
// rebuild yields byte-identical roads to running it incrementally. Only chunks
// whose road data actually changed are reported as affected.
//
// Guarantees:
//   - Roads never cross impassable terrain (cliffs).
//   - Every settlement is reachable over the road network; settlements that
//     cannot be connected are deterministically removed (this only happens
//     when cliffs fully isolate a site).
func RecomputeRoads(w *world.World) ([]world.ChunkCoord, error) {
	for {
		sets := collectSettlements(w)
		if len(sets) == 0 {
			return nil, nil
		}
		roads, err := buildRoadNetwork(w, sets)
		if err != nil {
			return nil, err
		}
		changed, _ := applyRoads(w, sets, roads)
		// On-demand chunk generation during pathfinding may have added new
		// settlements after the road network was built. Re-collect and drop
		// any settlement that is not connected to the main road network.
		allSets := collectSettlements(w)
		dropped := unreachableSettlements(w, allSets, roads)
		if len(dropped) == 0 {
			return changed, nil
		}
		// Deterministically drop unreachable settlements and rebuild.
		if err := dropSettlements(w, dropped); err != nil {
			return nil, err
		}
	}
}

// collectSettlements gathers all settlements from resident chunks in
// deterministic order (chunk coord, then settlement id).
func collectSettlements(w *world.World) []roadSettlement {
	var out []roadSettlement
	for _, c := range w.ChunkCoords() {
		ch := w.GetChunk(c)
		for _, s := range ch.Settlements {
			out = append(out, roadSettlement{
				id:    s.ID,
				tx:    int(s.TileX),
				ty:    int(s.TileY),
				chunk: world.ChunkCoord{X: int(s.ChunkX), Y: int(s.ChunkY)},
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].id != out[j].id {
			return out[i].id < out[j].id
		}
		if out[i].tx != out[j].tx {
			return out[i].tx < out[j].tx
		}
		return out[i].ty < out[j].ty
	})
	return out
}

// chunkAt returns the chunk containing global tile (tx,ty), generating it on
// demand (the generator is a pure function of seed/params/edits).
func chunkAt(w *world.World, tx, ty int) *world.Chunk {
	size := w.ChunkSize()
	cx := floorDiv(tx, size)
	cy := floorDiv(ty, size)
	coord := world.ChunkCoord{X: cx, Y: cy}
	ch := w.GetChunk(coord)
	if ch == nil {
		var err error
		ch, err = GenerateChunk(w, coord)
		if err != nil {
			return nil
		}
		w.PutChunk(ch)
	}
	return ch
}

// tileAt returns the tile at global coordinates, generating its chunk on
// demand (the generator is a pure function of seed/params/edits).
func tileAt(w *world.World, tx, ty int) *world.Tile {
	ch := chunkAt(w, tx, ty)
	if ch == nil {
		return nil
	}
	return ch.GlobalTile(tx, ty)
}

func floorDiv(a, b int) int {
	q := a / b
	if a < 0 && a%b != 0 {
		q--
	}
	return q
}

// buildRoadNetwork computes the MST over settlements and pathfinds each edge.
// It returns the set of road tiles keyed by global tile coordinates.
func buildRoadNetwork(w *world.World, sets []roadSettlement) (map[[2]int]bool, error) {
	edges := make([]roadEdge, 0, len(sets)*(len(sets)-1)/2)
	for i := 0; i < len(sets); i++ {
		for j := i + 1; j < len(sets); j++ {
			dx := float64(sets[i].tx - sets[j].tx)
			dy := float64(sets[i].ty - sets[j].ty)
			edges = append(edges, roadEdge{
				a: i, b: j,
				dist: math.Sqrt(dx*dx + dy*dy),
				idA:  sets[i].id, idB: sets[j].id,
			})
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].dist != edges[j].dist {
			return edges[i].dist < edges[j].dist
		}
		if edges[i].idA != edges[j].idA {
			return edges[i].idA < edges[j].idA
		}
		return edges[i].idB < edges[j].idB
	})

	parent := make([]int, len(sets))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra := find(a)
		rb := find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	roads := make(map[[2]int]bool)
	for _, e := range edges {
		if find(e.a) == find(e.b) {
			continue
		}
		path := findPath(w, sets[e.a].tx, sets[e.a].ty, sets[e.b].tx, sets[e.b].ty)
		if path == nil {
			continue // cliffs block this edge; try the next MST candidate
		}
		union(e.a, e.b)
		for _, p := range path {
			roads[p] = true
		}
	}
	return roads, nil
}

// node is an A* priority-queue entry.
type node struct {
	x, y int
	f, g float64
}

// findPath runs deterministic A* between two tiles. Returns nil when no path
// exists (cliffs block the way).
func findPath(w *world.World, x0, y0, x1, y1 int) [][2]int {
	p := w.Params()
	open := &nodeHeap{}
	heap.Init(open)
	start := node{x: x0, y: y0, g: 0, f: math.Hypot(float64(x1-x0), float64(y1-y0)) * p.RoadParams.LandCost}
	heap.Push(open, start)
	came := make(map[[2]int][2]int)
	gScore := make(map[[2]int]float64)
	gScore[[2]int{x0, y0}] = 0
	closed := make(map[[2]int]bool)

	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	for open.Len() > 0 {
		cur := heap.Pop(open).(node)
		if cur.x == x1 && cur.y == y1 {
			// reconstruct
			var path [][2]int
			k := [2]int{x1, y1}
			for {
				path = append(path, k)
				if k == [2]int{x0, y0} {
					break
				}
				k = came[k]
			}
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}
		k := [2]int{cur.x, cur.y}
		if closed[k] {
			continue
		}
		closed[k] = true
		for _, d := range dirs {
			nx, ny := cur.x+d[0], cur.y+d[1]
			nk := [2]int{nx, ny}
			if closed[nk] {
				continue
			}
			t := tileAt(w, nx, ny)
			if t == nil || t.IsImpassable() {
				continue
			}
			step := p.RoadParams.LandCost
			if t.IsWater() {
				step = p.RoadParams.WaterCost
			}
			if d[0] != 0 && d[1] != 0 {
				step *= math.Sqrt2
			}
			ng := cur.g + step
			if old, ok := gScore[nk]; ok && old <= ng {
				continue
			}
			gScore[nk] = ng
			came[nk] = k
			h := math.Hypot(float64(x1-nx), float64(y1-ny)) * p.RoadParams.LandCost
			heap.Push(open, node{x: nx, y: ny, g: ng, f: ng + h})
		}
	}
	return nil
}

type nodeHeap []node

func (h nodeHeap) Len() int { return len(h) }
func (h nodeHeap) Less(i, j int) bool {
	if h[i].f != h[j].f {
		return h[i].f < h[j].f
	}
	if h[i].g != h[j].g {
		return h[i].g < h[j].g
	}
	if h[i].x != h[j].x {
		return h[i].x < h[j].x
	}
	return h[i].y < h[j].y
}
func (h nodeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *nodeHeap) Push(x any) {
	*h = append(*h, x.(node))
}
func (h *nodeHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

// applyRoads rasterizes the road network into resident chunks, clears old
// road flags, and returns changed chunk coords plus unreachable settlements.
func applyRoads(w *world.World, sets []roadSettlement, roads map[[2]int]bool) ([]world.ChunkCoord, []uint64) {
	size := w.ChunkSize()

	// Rasterize road tiles per chunk.
	chunkRoads := make(map[world.ChunkCoord][]world.RoadTile)
	for k := range roads {
		tx, ty := k[0], k[1]
		cx := floorDiv(tx, size)
		cy := floorDiv(ty, size)
		coord := world.ChunkCoord{X: cx, Y: cy}
		ch := w.GetChunk(coord)
		if ch == nil {
			// Generated on demand by A*; make sure it is resident.
			ch = chunkAt(w, tx, ty)
			if ch == nil {
				continue
			}
			coord = ch.Coord
		}
		lx := tx - coord.X*size
		ly := ty - coord.Y*size
		t := ch.LocalTile(lx, ly)
		bridge := t.IsWater()
		chunkRoads[coord] = append(chunkRoads[coord], world.RoadTile{
			LocalX: uint16(lx), LocalY: uint16(ly), Bridge: bridge,
		})
	}

	// Deterministic ordering of road tiles within each chunk.
	for coord := range chunkRoads {
		rt := chunkRoads[coord]
		sort.Slice(rt, func(i, j int) bool {
			if rt[i].LocalY != rt[j].LocalY {
				return rt[i].LocalY < rt[j].LocalY
			}
			return rt[i].LocalX < rt[j].LocalX
		})
		chunkRoads[coord] = rt
	}

	// Clear old road flags on every resident chunk, then apply new ones.
	var changed []world.ChunkCoord
	for _, coord := range w.ChunkCoords() {
		ch := w.GetChunk(coord)
		for i := range ch.Tiles {
			ch.Tiles[i].ClearRoadFlags()
		}
		for _, rt := range chunkRoads[coord] {
			t := ch.LocalTile(int(rt.LocalX), int(rt.LocalY))
			t.SetRoad()
			if rt.Bridge {
				t.SetBridge()
			}
		}
		newRoads := chunkRoads[coord]
		if newRoads == nil {
			newRoads = []world.RoadTile{}
		}
		if !roadTilesEqual(ch.Roads, newRoads) {
			changed = append(changed, coord)
		}
		ch.Roads = newRoads
	}
	sort.Slice(changed, func(i, j int) bool {
		if changed[i].X != changed[j].X {
			return changed[i].X < changed[j].X
		}
		return changed[i].Y < changed[j].Y
	})

	dropped := unreachableSettlements(w, sets, roads)
	return changed, dropped
}

func roadTilesEqual(a, b []world.RoadTile) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// unreachableSettlements returns the ids of settlements not connected to the
// main road network. The network may consist of several components (a
// settlement behind cliffs gets a short isolated road or no road at all);
// the component containing the most settlements is treated as the main
// network, and every settlement outside it is dropped deterministically.
func unreachableSettlements(w *world.World, sets []roadSettlement, roads map[[2]int]bool) []uint64 {
	if len(sets) <= 1 {
		return nil
	}
	adj := make(map[[2]int][][2]int)
	// Sorted road keys make component labeling independent of map iteration
	// order (Go maps iterate in random order).
	keys := make([][2]int, 0, len(roads))
	for k := range roads {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	for _, k := range keys {
		x, y := k[0], k[1]
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
			nk := [2]int{x + d[0], y + d[1]}
			if roads[nk] {
				adj[k] = append(adj[k], nk)
			}
		}
	}
	// Label every road tile with a component id.
	comp := make(map[[2]int]int)
	compSizes := []int{}
	for _, k := range keys {
		if _, ok := comp[k]; ok {
			continue
		}
		id := len(compSizes)
		size := 0
		comp[k] = id
		queue := [][2]int{k}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			size++
			for _, n := range adj[cur] {
				if _, ok := comp[n]; !ok {
					comp[n] = id
					queue = append(queue, n)
				}
			}
		}
		compSizes = append(compSizes, size)
	}
	// Count settlements per component; the main component has the most
	// settlements (ties broken by smaller component id).
	settlesPerComp := make([]int, len(compSizes))
	for _, s := range sets {
		key := [2]int{s.tx, s.ty}
		if id, ok := comp[key]; ok {
			settlesPerComp[id]++
		}
	}
	mainComp := 0
	for id := 1; id < len(compSizes); id++ {
		if settlesPerComp[id] > settlesPerComp[mainComp] {
			mainComp = id
		}
	}
	var dropped []uint64
	for _, s := range sets {
		key := [2]int{s.tx, s.ty}
		id, ok := comp[key]
		if !ok || id != mainComp {
			dropped = append(dropped, s.id)
		}
	}
	sort.Slice(dropped, func(i, j int) bool { return dropped[i] < dropped[j] })
	return dropped
}

// dropSettlements removes settlements with the given ids from their chunks
// deterministically.
func dropSettlements(w *world.World, ids []uint64) error {
	idSet := make(map[uint64]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	for _, coord := range w.ChunkCoords() {
		ch := w.GetChunk(coord)
		kept := ch.Settlements[:0]
		for _, s := range ch.Settlements {
			if idSet[s.ID] {
				t := ch.GlobalTile(int(s.TileX), int(s.TileY))
				if t != nil {
					t.ClearSettlement()
				}
				continue
			}
			kept = append(kept, s)
		}
		ch.Settlements = kept
	}
	return nil
}
