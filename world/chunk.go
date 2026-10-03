package world

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
)

// Biome classifies a tile by temperature and moisture.
type Biome uint8

const (
	BiomeOcean Biome = iota
	BiomeBeach
	BiomeDesert
	BiomeSavanna
	BiomeGrassland
	BiomeForest
	BiomeRainforest
	BiomeTaiga
	BiomeTundra
	BiomeSnow
	BiomeCount
)

// String returns the biome name.
func (b Biome) String() string {
	switch b {
	case BiomeOcean:
		return "ocean"
	case BiomeBeach:
		return "beach"
	case BiomeDesert:
		return "desert"
	case BiomeSavanna:
		return "savanna"
	case BiomeGrassland:
		return "grassland"
	case BiomeForest:
		return "forest"
	case BiomeRainforest:
		return "rainforest"
	case BiomeTaiga:
		return "taiga"
	case BiomeTundra:
		return "tundra"
	case BiomeSnow:
		return "snow"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(b))
	}
}

// Tile is a single map cell. All fields are derived deterministically from
// the world seed, parameters and the tile's global coordinates.
type Tile struct {
	// Height in [0,1].
	Height float64
	// Moisture in [0,1].
	Moisture float64
	// Temperature in [0,1] (hot -> cold).
	Temperature float64
	// Slope is the max height difference to an 8-neighbour, in [0,1].
	Slope float64
	Biome Biome
	// Flags: bit0 = road, bit1 = bridge, bit2 = settlement, bit3 = impassable.
	flags uint8
}

// Flag bits.
const (
	flagRoad       uint8 = 1 << 0
	flagBridge     uint8 = 1 << 1
	flagSettlement uint8 = 1 << 2
	flagImpassable uint8 = 1 << 3
)

// SetRoad marks the tile as crossed by a road.
func (t *Tile) SetRoad() { t.flags |= flagRoad }

// SetBridge marks the tile as a bridge crossing water.
func (t *Tile) SetBridge() { t.flags |= flagBridge }

// SetSettlement marks the tile as a settlement site.
func (t *Tile) SetSettlement() { t.flags |= flagSettlement }

// SetImpassable marks the tile as impassable for roads.
func (t *Tile) SetImpassable() { t.flags |= flagImpassable }

// ClearRoadFlags removes road and bridge marks.
func (t *Tile) ClearRoadFlags() { t.flags &^= flagRoad | flagBridge }

// ClearSettlement removes the settlement mark.
func (t *Tile) ClearSettlement() { t.flags &^= flagSettlement }

// HasRoad reports whether a road crosses the tile.
func (t Tile) HasRoad() bool { return t.flags&flagRoad != 0 }

// HasBridge reports whether the road crosses water here.
func (t Tile) HasBridge() bool { return t.flags&flagBridge != 0 }

// HasSettlement reports whether a settlement occupies the tile.
func (t Tile) HasSettlement() bool { return t.flags&flagSettlement != 0 }

// IsImpassable reports whether the tile is impassable for roads (cliff).
func (t Tile) IsImpassable() bool { return t.flags&flagImpassable != 0 }

// IsWater reports whether the tile is ocean.
func (t Tile) IsWater() bool { return t.Biome == BiomeOcean }

// Settlement is a deterministic settlement site. Its ID is a pure hash of the
// seed, chunk coordinates and candidate index, so it is stable across runs.
type Settlement struct {
	ID     uint64
	TileX  int32
	TileY  int32
	ChunkX int32
	ChunkY int32
	Score  float64
	Name   string
}

// RoadTile marks a road crossing a local tile.
type RoadTile struct {
	LocalX uint16
	LocalY uint16
	Bridge bool
}

// Chunk is a square region of the world. Chunks are produced by a pure
// generation function of (seed, params, edits); see GenerateChunk.
type Chunk struct {
	Coord            ChunkCoord
	Size             int
	AlgorithmVersion int
	Seed             int64

	Tiles       []Tile // length Size*Size, index = y*Size + x
	Settlements []Settlement
	Roads       []RoadTile
}

// ChunkCoord identifies a chunk in chunk space.
type ChunkCoord struct {
	X int
	Y int
}

// TileIndex returns the flat index of a local tile.
func (c *Chunk) TileIndex(lx, ly int) int { return ly*c.Size + lx }

// LocalTile returns the tile at local coordinates.
func (c *Chunk) LocalTile(lx, ly int) *Tile {
	return &c.Tiles[c.TileIndex(lx, ly)]
}

// GlobalTile returns the tile at global tile coordinates, or nil if the
// coordinates are outside the chunk.
func (c *Chunk) GlobalTile(tx, ty int) *Tile {
	lx := tx - c.Coord.X*c.Size
	ly := ty - c.Coord.Y*c.Size
	if lx < 0 || ly < 0 || lx >= c.Size || ly >= c.Size {
		return nil
	}
	return &c.Tiles[c.TileIndex(lx, ly)]
}

// MarshalBinary encodes the chunk in a fixed, deterministic byte layout.
// The encoding contains no maps and no timestamps, so equal chunks always
// produce equal bytes.
func (c *Chunk) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	if err := c.encode(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *Chunk) encode(buf *bytes.Buffer) error {
	write := func(v any) error { return binary.Write(buf, binary.LittleEndian, v) }
	if err := write(uint32(0xC17E)); err != nil { // magic
		return err
	}
	if err := write(uint16(1)); err != nil { // encoding version
		return err
	}
	if err := write(int64(c.Seed)); err != nil {
		return err
	}
	if err := write(int32(c.AlgorithmVersion)); err != nil {
		return err
	}
	if err := write(int32(c.Coord.X)); err != nil {
		return err
	}
	if err := write(int32(c.Coord.Y)); err != nil {
		return err
	}
	if err := write(int32(c.Size)); err != nil {
		return err
	}
	for i := range c.Tiles {
		t := &c.Tiles[i]
		if err := write(t.Height); err != nil {
			return err
		}
		if err := write(t.Moisture); err != nil {
			return err
		}
		if err := write(t.Temperature); err != nil {
			return err
		}
		if err := write(t.Slope); err != nil {
			return err
		}
		if err := write(uint8(t.Biome)); err != nil {
			return err
		}
		if err := write(t.flags); err != nil {
			return err
		}
	}
	if err := write(uint32(len(c.Settlements))); err != nil {
		return err
	}
	for i := range c.Settlements {
		s := &c.Settlements[i]
		if err := write(s.ID); err != nil {
			return err
		}
		if err := write(s.TileX); err != nil {
			return err
		}
		if err := write(s.TileY); err != nil {
			return err
		}
		if err := write(s.ChunkX); err != nil {
			return err
		}
		if err := write(s.ChunkY); err != nil {
			return err
		}
		if err := write(s.Score); err != nil {
			return err
		}
		if err := write(uint16(len(s.Name))); err != nil {
			return err
		}
		if _, err := buf.WriteString(s.Name); err != nil {
			return err
		}
	}
	if err := write(uint32(len(c.Roads))); err != nil {
		return err
	}
	for i := range c.Roads {
		r := &c.Roads[i]
		if err := write(r.LocalX); err != nil {
			return err
		}
		if err := write(r.LocalY); err != nil {
			return err
		}
		var b uint8
		if r.Bridge {
			b = 1
		}
		if err := write(b); err != nil {
			return err
		}
	}
	return nil
}

// UnmarshalBinary decodes a chunk produced by MarshalBinary.
func (c *Chunk) UnmarshalBinary(data []byte) error {
	buf := bytes.NewReader(data)
	read := func(v any) error { return binary.Read(buf, binary.LittleEndian, v) }
	var magic uint32
	if err := read(&magic); err != nil {
		return err
	}
	if magic != 0xC17E {
		return errors.New("chunk: bad magic")
	}
	var encVersion uint16
	if err := read(&encVersion); err != nil {
		return err
	}
	if encVersion != 1 {
		return fmt.Errorf("chunk: unsupported encoding version %d", encVersion)
	}
	if err := read(&c.Seed); err != nil {
		return err
	}
	var i32 int32
	if err := read(&i32); err != nil {
		return err
	}
	c.AlgorithmVersion = int(i32)
	if err := read(&i32); err != nil {
		return err
	}
	c.Coord.X = int(i32)
	if err := read(&i32); err != nil {
		return err
	}
	c.Coord.Y = int(i32)
	if err := read(&i32); err != nil {
		return err
	}
	c.Size = int(i32)
	n := c.Size * c.Size
	c.Tiles = make([]Tile, n)
	for i := range c.Tiles {
		t := &c.Tiles[i]
		if err := read(&t.Height); err != nil {
			return err
		}
		if err := read(&t.Moisture); err != nil {
			return err
		}
		if err := read(&t.Temperature); err != nil {
			return err
		}
		if err := read(&t.Slope); err != nil {
			return err
		}
		var b uint8
		if err := read(&b); err != nil {
			return err
		}
		t.Biome = Biome(b)
		if err := read(&t.flags); err != nil {
			return err
		}
	}
	var ns uint32
	if err := read(&ns); err != nil {
		return err
	}
	c.Settlements = make([]Settlement, ns)
	for i := range c.Settlements {
		s := &c.Settlements[i]
		if err := read(&s.ID); err != nil {
			return err
		}
		if err := read(&s.TileX); err != nil {
			return err
		}
		if err := read(&s.TileY); err != nil {
			return err
		}
		if err := read(&s.ChunkX); err != nil {
			return err
		}
		if err := read(&s.ChunkY); err != nil {
			return err
		}
		if err := read(&s.Score); err != nil {
			return err
		}
		var ln uint16
		if err := read(&ln); err != nil {
			return err
		}
		name := make([]byte, ln)
		if _, err := buf.Read(name); err != nil {
			return err
		}
		s.Name = string(name)
	}
	var nr uint32
	if err := read(&nr); err != nil {
		return err
	}
	c.Roads = make([]RoadTile, nr)
	for i := range c.Roads {
		r := &c.Roads[i]
		if err := read(&r.LocalX); err != nil {
			return err
		}
		if err := read(&r.LocalY); err != nil {
			return err
		}
		var b uint8
		if err := read(&b); err != nil {
			return err
		}
		r.Bridge = b != 0
	}
	return nil
}

// StableHash returns a deterministic 64-bit hash of the chunk content.
func (c *Chunk) StableHash() (uint64, error) {
	data, err := c.MarshalBinary()
	if err != nil {
		return 0, err
	}
	h := fnv.New64a()
	h.Write(data)
	return h.Sum64(), nil
}
