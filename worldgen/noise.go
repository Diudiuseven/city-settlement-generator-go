package worldgen

// All noise math is integer fixed-point. No float transcendentals are
// used, so identical inputs are bit-identical on every architecture.

const (
	noiseGridMask = uint64(1<<16) - 1 // corner values are 16 bit
	noiseFBMOct   = 4
)

// hashU64 is the deterministic integer hash primitive
// (splitmix64 finalizer).
func hashU64(z uint64) uint64 {
	z += 0x9e3779b97f4a7c15
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// mix64 combines two 64 bit values into one.
func mix64(a, b uint64) uint64 {
	return hashU64(a ^ hashU64(b))
}

// latticeHash returns the deterministic 16 bit corner value at integer
// lattice point (gx,gy) for one noise layer.
func latticeHash(seed, layer, gx, gy uint64) uint64 {
	h := hashU64(seed)
	h = mix64(h, layer)
	h = mix64(h, uint64(int64(gx)))
	h = mix64(h, uint64(int64(gy)))
	return h & noiseGridMask
}

// noiseState is the derived integer-noise configuration for a world.
type noiseState struct {
	seed  uint64
	scale uint64
}

func newNoiseState(seed int64, scale uint64) noiseState {
	return noiseState{seed: uint64(seed), scale: scale}
}

// smoothstep is the quintic 6t^5-15t^4+10t^3 ease, evaluated in 16 bit
// fixed point. t is in [0,65536).
func smoothstep(t uint64) uint64 {
	// s = t/65536 in [0,1); use 32 bit fixed point for the powers.
	const q = uint64(1) << 16
	s := t // [0,q)
	s2 := (s * s) / q
	s3 := (s2 * s) / q
	s4 := (s3 * s) / q
	s5 := (s4 * s) / q
	// 6*s5 - 15*s4 + 10*s3  (coefficients fit: max value 6*65535)
	v := 6*s5 - 15*s4 + 10*s3
	return v / q // back to [0,q)
}

// lerp16 linearly interpolates between two 16 bit values; f is [0,65536).
func lerp16(a, b, f uint64) uint64 {
	return a + ((b-a)*f)/(1<<16)
}

// valueNoise2D returns smooth deterministic value noise at integer
// cell coordinates, in [0,65535].
func (n noiseState) valueNoise2D(x, y int, layerSeed uint64) uint32 {
	fx := uint64(x) / n.scale
	fy := uint64(y) / n.scale
	rx := uint64(x) % n.scale
	ry := uint64(y) % n.scale
	gx := fx
	gy := fy
	v00 := latticeHash(n.seed, layerSeed, gx, gy)
	v10 := latticeHash(n.seed, layerSeed, gx+1, gy)
	v01 := latticeHash(n.seed, layerSeed, gx, gy+1)
	v11 := latticeHash(n.seed, layerSeed, gx+1, gy+1)
	// scale the intra-cell remainder to 16 bit
	tx := rx * (1 << 16) / n.scale
	ty := ry * (1 << 16) / n.scale
	sx := smoothstep(tx)
	sy := smoothstep(ty)
	top := lerp16(v00, v10, sx)
	bot := lerp16(v01, v11, sx)
	return uint32(lerp16(top, bot, sy))
}

// fbm stacks value-noise octaves into a value in [0,65535]. Octave i
// samples the noise at frequency 2^i with inverse-amplitude weight
// (2^(oct-i)); the weighted average is renormalized to 16 bit.
func (n noiseState) fbm(x, y int, baseLayer uint64) uint16 {
	var sum, weight uint64
	for i := 0; i < noiseFBMOct; i++ {
		// Higher octaves use tighter lattices (frequency 2^i).
		scale := n.scale >> uint(i)
		if scale < 2 {
			scale = 2
		}
		v := uint64(n.valueNoise2DScaled(x, y, scale, baseLayer+uint64(i)))
		w := uint64(1) << uint(noiseFBMOct-1-i)
		sum += v * w
		weight += w * noiseGridMask
	}
	return uint16(sum * noiseGridMask / weight)
}

// valueNoise2DScaled is valueNoise2D with an explicit scale.
func (n noiseState) valueNoise2DScaled(x, y int, scale, layerSeed uint64) uint32 {
	sub := noiseState{seed: n.seed, scale: scale}
	return sub.valueNoise2D(x, y, layerSeed)
}

// fbmHeight stacks value-noise octaves into an elevation in [0,65535].
func (n noiseState) fbmHeight(x, y int) Height {
	return Height(n.fbm(x, y, 1))
}

// fbmMoisture stacks value-noise octaves into a moisture value.
func (n noiseState) fbmMoisture(x, y int) Moisture {
	return Moisture(n.fbm(x, y, 100))
}
