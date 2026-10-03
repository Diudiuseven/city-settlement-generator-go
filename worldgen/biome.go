package worldgen

import "github.com/Diudiuseven/city-settlement-generator-go/world"

// classifyBiome assigns a biome from height, temperature and moisture.
// The classification is a pure function of its inputs.
func classifyBiome(height, temperature, moisture, seaLevel float64) world.Biome {
	if height < seaLevel-0.02 {
		return world.BiomeOcean
	}
	if height < seaLevel+0.02 {
		return world.BiomeBeach
	}
	// High altitudes are alpine regardless of moisture.
	if height > 0.86 {
		return world.BiomeSnow
	}
	switch {
	case temperature >= 0.75: // tropical
		switch {
		case moisture < 0.25:
			return world.BiomeDesert
		case moisture < 0.5:
			return world.BiomeSavanna
		default:
			return world.BiomeRainforest
		}
	case temperature >= 0.55: // warm temperate
		switch {
		case moisture < 0.3:
			return world.BiomeGrassland
		case moisture < 0.65:
			return world.BiomeForest
		default:
			return world.BiomeRainforest
		}
	case temperature >= 0.35: // cool temperate
		switch {
		case moisture < 0.35:
			return world.BiomeGrassland
		case moisture < 0.7:
			return world.BiomeForest
		default:
			return world.BiomeTaiga
		}
	case temperature >= 0.2: // cold
		if moisture < 0.5 {
			return world.BiomeTundra
		}
		return world.BiomeTaiga
	default: // polar
		return world.BiomeSnow
	}
}
