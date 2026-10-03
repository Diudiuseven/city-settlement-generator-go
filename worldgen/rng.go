package worldgen

import "math"

// splitmix64 is the deterministic mixer at the heart of the generator.
// Every random value is a pure function of (seed, coordinates, salt); the
// generator holds no mutable state, so results are independent of call order,
// traversal order and concurrency.
func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// hash2 mixes a seed and two integer coordinates into a uniform uint64.
func hash2(seed uint64, x, y int64) uint64 {
	h := splitmix64(seed)
	h = splitmix64(h + uint64(x)*0x9e3779b97f4a7c15)
	h = splitmix64(h + uint64(y)*0xbf58476d1ce4e5b9)
	return h
}

// hash3 mixes a seed and three integer coordinates.
func hash3(seed uint64, x, y, z int64) uint64 {
	h := hash2(seed, x, y)
	return splitmix64(h + uint64(z)*0x94d049bb133111eb)
}

// hashFloat returns a deterministic float64 in [0,1).
func hashFloat(seed uint64, x, y int64) float64 {
	return float64(hash2(seed, x, y)>>11) / float64(uint64(1)<<53)
}

// valueNoise2D returns smooth value noise in [-1,1]. Lattice values are
// derived from the seed and the integer lattice coordinates, so the field is
// a pure function of (seed, x, y): it is seamless across chunk borders and
// identical no matter which chunk is sampled first.
func valueNoise2D(seed uint64, x, y float64) float64 {
	xi := math.Floor(x)
	yi := math.Floor(y)
	tx := x - xi
	ty := y - yi
	// Quintic interpolation: C2 continuous, no visible lattice seams.
	u := tx * tx * tx * (tx*(tx*6-15) + 10)
	v := ty * ty * ty * (ty*(ty*6-15) + 10)

	ix := int64(xi)
	iy := int64(yi)
	v00 := hashFloat(seed, ix, iy)*2 - 1
	v10 := hashFloat(seed, ix+1, iy)*2 - 1
	v01 := hashFloat(seed, ix, iy+1)*2 - 1
	v11 := hashFloat(seed, ix+1, iy+1)*2 - 1

	a := v00 + (v10-v00)*u
	b := v01 + (v11-v01)*u
	return a + (b-a)*v
}

// fbm returns fractal Brownian motion in [-1,1] by summing octaves of value
// noise. It is a pure function of (seed, x, y).
func fbm(seed uint64, x, y float64, octaves int, lacunarity, gain float64) float64 {
	amp := 0.5
	freq := 1.0
	sum := 0.0
	norm := 0.0
	for i := 0; i < octaves; i++ {
		sum += amp * valueNoise2D(seed, x*freq, y*freq)
		norm += amp
		amp *= gain
		freq *= lacunarity
	}
	return sum / norm
}

// ridgeNoise returns ridged noise in [0,1], used for mountain ranges.
func ridgeNoise(seed uint64, x, y float64, octaves int) float64 {
	n := fbm(seed, x, y, octaves, 2.1, 0.5)
	r := 1 - math.Abs(n)
	return r * r
}

// jitter returns a deterministic pseudo-random offset in [0,1) for a point
// (x, y) in a chunk.
func jitter(seed uint64, chunkX, chunkY, salt int64) (float64, float64) {
	jx := hashFloat(seed, chunkX*257+salt, chunkY*257)
	jy := hashFloat(seed, chunkX*257, chunkY*257+salt)
	return jx, jy
}
