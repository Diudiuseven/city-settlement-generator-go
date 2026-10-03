package worldgen

// planSettlements deterministically selects settlement sites.
//
// Sites live on a jittered square lattice. The jitter radius r and the
// lattice spacing s satisfy 2r+2 <= s, hence two sites from different
// lattice cells are always at least 2 cells apart center-to-center and
// (when params are normalized) at least MinSettlementDist apart:
// acceptance of one site can never depend on another lattice cell.
//
// The accept decision for lattice cell (ix,iy) depends only on terrain
// inside a bounded neighborhood of that cell, so an edit outside the
// neighborhood cannot change the site. Iteration order is the canonical
// ix-major raster order, so the output list is identical regardless of
// scheduling.
func planSettlements(p Params, view terrainView) []Settlement {
	s := p.SettlementSpacing
	if s <= 0 {
		return nil
	}
	var out []Settlement
	for iy := 0; iy*s < p.Height; iy++ {
		for ix := 0; ix*s < p.Width; ix++ {
			bx := ix*s + s/2
			by := iy*s + s/2
			dx, dy := jitterOffset(p, ix, iy)
			x, y := bx+dx, by+dy
			if x < 0 || y < 0 || x >= p.Width || y >= p.Height {
				continue
			}
			score := settlementScore(view, p, x, y)
			if score < p.MinSuitability {
				continue
			}
			homeCX := x / p.ChunkSize
			homeCY := y / p.ChunkSize
			out = append(out, Settlement{
				ID:          settlementID(p, ix, iy),
				X:           x,
				Y:           y,
				Home:        ChunkCoord{CX: homeCX, CY: homeCY},
				Suitability: score,
				Name:        settlementName(p, ix, iy),
			})
		}
	}
	return out
}

// settlementID is a stable dense identifier derived solely from lattice
// coordinates and the spacing. Terrain edits can never change an ID;
// an edit can only make an existing site disappear (score fell below
// the threshold) in its bounded local neighborhood.
func settlementID(p Params, ix, iy int) uint64 {
	cols := (p.Width + p.SettlementSpacing - 1) / p.SettlementSpacing
	return uint64(iy)*uint64(cols) + uint64(ix) + 1
}

// jitterOffset returns the deterministic lattice-cell jitter in
// [-Jitter, Jitter] on each axis.
func jitterOffset(p Params, ix, iy int) (dx, dy int) {
	base := mix64(uint64(p.WorldSeed), 0x5E77)
	hx := mix64(base, uint64(int64(ix))*0x9E3779B1+uint64(int64(iy)))
	hy := mix64(base, uint64(int64(iy))*0xC2B2AE3D+uint64(int64(ix)))
	r := p.SettlementJitter
	jx := int(hx%uint64(2*r+1)) - r
	jy := int(hy%uint64(2*r+1)) - r
	return jx, jy
}

// settlementScore scores a candidate in [0,65535] from a fixed 5x5
// window. Terrain is preferred for settlements when it is flat,
// passable dry land with moderate moisture; water and mountains score
// near zero.
func settlementScore(view terrainView, p Params, x, y int) uint16 {
	const rad = 2
	var sum uint64
	var n uint64
	var center Cell
	for dy := -rad; dy <= rad; dy++ {
		for dx := -rad; dx <= rad; dx++ {
			c := view.cellAt(x+dx, y+dy)
			if dx == 0 && dy == 0 {
				center = c
			}
			if !Passable(c.Biome) {
				return 0
			}
			sum += uint64(landScore(c, p))
			n++
		}
	}
	// Require the center itself to be comfortably above water and
	// below the mountains.
	if !Passable(center.Biome) {
		return 0
	}
	return uint16(sum / n)
}

// landScore maps one passable cell to a settlement score.
func landScore(c Cell, p Params) uint16 {
	if !Passable(c.Biome) {
		return 0
	}
	// Prefer the middle of the land elevation band.
	lo := uint64(p.WaterThreshold)
	hi := uint64(p.MountainThreshold)
	mid := (lo + hi) / 2
	h := uint64(c.Height)
	d := uint64(0)
	if h > mid {
		d = h - mid
	} else {
		d = mid - h
	}
	elev := uint64(32768) - d/2
	if elev > 65535 {
		elev = 0
	}
	// Prefer moderate moisture [16384,49152].
	m := uint64(c.Moisture)
	mois := uint64(32768)
	switch {
	case m < 16384:
		mois = m * 2
	case m > 49152:
		if d2 := m - 49152; d2 < 32768 {
			mois = 32768 - d2
		} else {
			mois = 0
		}
	}
	v := (elev + mois) / 2
	if v > 65535 {
		v = 65535
	}
	return uint16(v)
}

// settlementName derives a stable pronounceable name from the lattice
// coordinates and the (terrain-independent) name salt.
func settlementName(p Params, ix, iy int) string {
	const syllables = "bdfglmnprstvwz"
	const vowels = "aeiou"
	h := mix64(uint64(p.WorldSeed)^uint64(p.NameSalt), 0x4A4D)
	h = mix64(h, uint64(int64(ix))*0x9E3779B97F4A7C15)
	h = mix64(h, uint64(int64(iy))*0xD1B54A32D192ED03)
	lenSyll := int(h%2) + 2 // 2 or 3 syllables
	b := make([]byte, 0, lenSyll*2)
	for i := 0; i < lenSyll; i++ {
		h = hashU64(h + uint64(i)*0x100000001B3)
		b = append(b, syllables[int(h>>8)%len(syllables)])
		b = append(b, vowels[int(h>>16)%len(vowels)])
	}
	b[0] = b[0] - ('a' - 'A') // capitalize
	return string(b)
}
