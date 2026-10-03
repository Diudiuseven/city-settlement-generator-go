# 算法版本与兼容规则（Versioning & Compatibility）

## 核心承诺

> 同一份世界种子与生成参数，在任何机器、任何时间重复运行，
> 必须得到逐字节一致的地图；旧参数组合下的地图必须永久可复现。

为实现这一点，地形算法采用**只增不改的版本注册表**：

```go
type terrainFn func(p Params, x, y int) (Height, Moisture)

var algorithmRegistry = map[uint16]terrainFn{}
```

- 每个版本号绑定一个冻结的**纯点函数**；v1 已在 `init()` 中注册
  （`registerAlgorithm(1, terrain1)`）。
- 已注册的函数**永不修改**、**永不删除**。任何行为变化只能通过
  注册一个**新版本号**引入（例如 `registerAlgorithm(2, terrain2)`）。
- 重复注册同一版本号会在包初始化时 panic（属编程错误）。
- `Params.AlgorithmVersion=0` 归一化为当前默认版本 `DefaultAlgorithmVersion=1`；
  未注册版本被 `Validate` 拒绝，世界无法创建。

## 世界绑定算法版本

每个 `World` 在创建时固化 `Params.AlgorithmVersion`，并携带两个摘要：

- `TerrainDigest`：地形身份（算法版本 + 全部地形相关参数）；
- `Digest`：完整参数身份。

`EncodeWorld` 把这两个摘要写入文件头，配合编辑日志即可：

1. 校验已加载世界与当前参数是否一致；
2. 在未来加入 v2/v3 后，仍能识别并重建旧的 v1 世界。

## 变更分类与规则

| 变更类型 | 例子 | 规则 |
|---|---|---|
| 冻结算法的实现“改动” | 改噪声八度、改走廊形状、改阈值含义 | **禁止**。必须新开版本 |
| 新增算法版本 | v2：换成域扭曲噪声 | 新增注册表项；旧 v1 世界不受影响 |
| 地形相关参数变化 | 改种子、阈值、尺寸、块大小、间距 | 会改变已有区域；不能就地 `UpdateParams`，需新建版本化世界 |
| 地形无关参数变化 | `NameSalt`、`MinSuitability`、`RoadCostScale` | 允许 `UpdateParams`；已生成地形块逐字节不变，只重算派生层 |
| 新增地形无关参数 | 新的道路风格开关 | 不进 `TerrainDigest`，追加到 `Digest`，旧世界地形不受影响 |

## “修改与地形无关的参数不得改动已有区域”

`World.UpdateParams(p2)`：

1. 归一化并校验 `p2`；
2. 比较 `p2.TerrainDigest()` 与当前世界；不同则返回
   `ErrTerrainParamsChanged`，世界不变；
3. 相同则替换参数，只重新派生聚落与道路层；`chunks` 不重算、不改写。

## 序列化格式

`EncodeWorld` 输出规范大端二进制，同逻辑世界 ⇒ 同字节：

```
"WGEN" | 序列化格式版本(u16)
| TerrainDigest(32B) | Digest(32B)
| 编辑日志：数量(u32)，每条 [x u32][y u32][flags u16][height?][moisture?]，按 (y,x) 排序
| 块：数量(u32)，每块 [cx u32][cy u32][size u32] 后接 size² 个 [H u16][M u16][B u16]，按坐标排序
| 聚落：数量(u32)，每个 [id u64][x u32][y u32][score u16][nameLen u32][name]
| 道路：数量(u32)，每条 [a u64][b u64][cost u64][点数 u32] 点序列 [x u32][y u32]
```

## 测试如何锁死兼容承诺

- `TestTerrainDigestPinned`：锁定 v1 地形摘要的黄金值；
- `TestV1ReproducibilityPins`：锁定标准测试世界完整编码的 SHA-256；
- `TestRegistryIsAppendOnly`：禁止重复注册；
- `TestUnknownVersionRejected`：未知版本不可创建世界；
- `TestDerivedParamsDoNotChangeTerrain`：地形无关参数不改已有单元；
- `TestTerrainParamsUpdateRejected`：地形参数变更被拒绝就地应用；
- `TestOldVersionWorldStaysReproducible`：旧版本摘要在新版本出现后保持稳定。
