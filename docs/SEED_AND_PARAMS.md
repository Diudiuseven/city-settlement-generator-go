# 种子语义与参数含义（Seed Semantics & Parameters）

## 种子语义

`WorldSeed int64` 是世界中**每一个随机决策**的唯一根。它通过
splitmix64 终态化哈希（`hashU64` / `mix64`）派生出：

- value noise 每个整数格点的角点值；
- FBM 各八度（height / moisture 使用不同的层种子常量）；
- 每个聚落格点的 `(dx, dy)` 抖动；
- 聚落名字的音节选择。

语义约定：

1. **相同 `(WorldSeed, Params, edit log)` ⇒ 完全相同的地图。**
   与并发度、遍历顺序、调度、机器、进程重启无关，逐字节一致。
2. 不同 `WorldSeed` 产生不同的噪声场、抖动与命名。
3. 哈希全部是整数运算，输入坐标按位解释为 `int64` 后再哈希，
   负坐标也是良定义的。
4. 种子本身不编码算法版本；算法版本由 `AlgorithmVersion` 单独固定
   （见 [VERSIONING.md](VERSIONING.md)）。
5. `NameSalt` 只影响聚落命名，是“地形无关参数”，不会改变任何单元。

## 参数含义

| 参数 | 作用 | 是否影响地形 |
|---|---|---|
| `AlgorithmVersion` | 选择冻结的地形点函数；0 归一化为 1 | 是 |
| `Width`, `Height` | 世界单元尺寸（须被块尺寸整除） | 是 |
| `ChunkSize` | 块边长（单元） | 是（分块身份） |
| `WorldSeed` | 所有随机决策的根 | 是 |
| `NoiseScale` | 基础 value-noise 格距（单元），越大地形越平缓 | 是 |
| `MountainThreshold` | 海拔 ≥ 该值为山地（不可通行） | 是 |
| `WaterThreshold` | 海拔 < 该值为水域；其 0.8 倍以下为深水 | 是 |
| `BackboneSpacing` | 纵向可通行主干走廊的列间距 | 是（走廊塑形） |
| `SettlementSpacing` | 聚落栅格间距 S | 是（安全区/引道塑形） |
| `SettlementJitter` | 栅格抖动半径 r，须满足 `2r+2 <= S` | 是（安全区/引道塑形） |
| `NameSalt` | 聚落命名扰动 | **否** |
| `MinSuitability` | 聚落接受的最低适宜度 | **否**（仅派生层） |
| `MinSettlementDist2` | 聚落中心最小距离平方；为 0 时由 S、r 自动导出 | **否**（结构已保证） |
| `RoadCostScale` | 道路代价线性系数，只改权值不改路径存在性 | **否**（仅派生层） |

> “是否影响地形”决定该参数是否进入 `Params.TerrainDigest()`。
> 只有影响地形的参数变化才会改变已有区域；地形无关参数变化
> 只重算聚落/道路派生层，已生成块逐字节不变。

### 默认值（`DefaultParams`，v1）

256×256 世界、64×64 块；`NoiseScale=40`；山地阈值 46000、水面 19000；
主干间距 48；聚落栅格 24、抖动 4；最低适宜度 16000；道路系数 1。

### 归一化（`Normalize`）与校验（`Validate`）

- 零值字段用默认值填充；`MinSettlementDist2` 自动取
  `(SettlementSpacing - 2·SettlementJitter)^2`；
- 校验：尺寸为正且整除；`NoiseScale>=2`；
  `WaterThreshold < MountainThreshold`；`BackboneSpacing>1`；
  `SettlementSpacing>=3`；抖动不变式；算法版本必须已注册。

## 摘要（digest）

- `Params.TerrainDigest()`：对“可能影响地形单元”的字段（含算法版本）
  按大端序规范序列化后取 SHA-256。它是旧世界钉住的地形身份；
  v1 的序列化字段顺序被测试永久锁定。
- `Params.Digest()`：覆盖全部参数（含仅派生层字段）。

`World.UpdateParams` 只允许地形无关参数变更（`TerrainDigest` 不变），
否则返回 `ErrTerrainParamsChanged`——要换地形请新建一个版本化世界，
旧世界继续钉在自己的 `TerrainDigest` 上保持可复现。
