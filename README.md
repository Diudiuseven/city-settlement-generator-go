# city-settlement-generator-go

确定性的**城邦与聚落地形生成**库（纯 Go、仅标准库）。给模拟与可视化
工具产出**可复现**的世界：同一份世界种子 + 生成参数，在任何机器、
任意并发度、进程重启后重复运行都得到**逐字节一致**的地图。

## 能力一览

1. **完全确定性**：结果只由种子与参数决定，与并发调度、遍历顺序、
   并发度、运行环境无关；全部使用整数定点噪声，无浮点超越函数。
2. **可配置分块、块间无缝**：地形是纯点函数，高度/湿度/生态在
   块边界连续；单独生成一块与连同邻块一起生成完全相同。
3. **聚落选址**：抖动栅格 + 地形/生态适宜度约束；结构保证最小间距、
   不重叠；同种子稳定可复现。
4. **道路与连通性**：确定性 A* + Kruskal MST，道路只走可通行地形、
   禁止穿墙角；结构性走廊保证无编辑地形上任何聚落都可达；
   增量重算的连通关系与从零重建一致。
5. **局部编辑**：字段级覆盖层，编辑只重算受影响块；提交顺序无关、
   重复提交幂等；编辑后世界与从零重建（seed+params+edit log）
   逐字节对齐；会断连的编辑批次被原子拒绝。
6. **多块并行、取消安全**：有界并行渲染，取消/超时不发布半成品，
   慢块不阻塞其它块，结果与并发度无关。
7. **算法版本化**：地图记录绑定的算法版本；只增不改的版本注册表
   保证旧参数组合永久可复现；地形无关参数变更不改动已有区域。
8. **自动化测试与文档**：覆盖确定性、接缝、增量/重建一致、
   并发与取消、版本兼容。

## 快速开始

```go
import (
    "context"
    "github.com/Diudiuseven/city-settlement-generator-go/worldgen"
)

p := worldgen.DefaultParams()          // 256x256, 64x64 chunks, seed=1
w, err := worldgen.NewWorld(p)         // 归一化 + 校验，固定算法版本
if err != nil { /* 参数非法 */ }

ctx := context.Background()

// 按需、并行生成某些块（纯函数，块间无依赖，天然无缝）
err = w.GenerateChunks(ctx,
    []worldgen.ChunkCoord{{0, 0}, {1, 0}},
    worldgen.GenerateOptions{Concurrency: 8})

// 局部编辑（顺序无关、幂等；会断连的批次整体失败、不留半成品）
h := worldgen.Height(30000)
err = w.ApplyEdits(ctx, []worldgen.CellEdit{
    {X: 10, Y: 12, SetHeight: &h},
}, worldgen.GenerateOptions{Concurrency: 4})

// 从零重建整张地图：与增量维护的结果逐字节一致
err = w.Rebuild(ctx, worldgen.GenerateOptions{Concurrency: 1})

_ = worldgen.EncodeWorld(w)            // 规范大端序列化
```

## 目录

- `worldgen/` 生成库
  - `params.go` 参数、归一化/校验、摘要
  - `version.go` 算法版本注册表
  - `noise.go` 整数定点 value noise / FBM
  - `terrain.go` 点函数、生态分类、走廊塑形、块渲染、并行生成
  - `settlement.go` 抖动栅格选址、适宜度、稳定 ID/命名
  - `roads.go` 确定性 A*、MST、连通维护
  - `edits.go` 不可变编辑覆盖层
  - `world.go` 编排：原子提交、编辑、重建、序列化
- `docs/`
  - [WORLD_MODEL.md](docs/WORLD_MODEL.md) 世界模型
  - [SEED_AND_PARAMS.md](docs/SEED_AND_PARAMS.md) 种子语义与参数含义
  - [VERSIONING.md](docs/VERSIONING.md) 版本兼容规则

## 运行测试

```bash
go test ./...
go test -race ./worldgen/
```

## 确定性是怎么做到的

- 地形 = 版本化纯点函数：单元值只依赖 `(seed, params, x, y)`，
  与块、邻居、顺序、并发无关 —— 无缝与可并行的根本；
- 所有随机来自 splitmix64 整数哈希，噪声用整数定点插值与 FBM；
- 聚落按固定栅格扫描顺序、稳定 ID；道路 A* 用 `(f, 单元键)` 全序
  打破平局，MST 按 `(cost, a, b)` 全序选边；
- 派生层从点函数视图计算，不依赖“哪些块恰好已生成”；
- 编辑发布不可变，取消只丢弃本地结果后在调用方原子提交。
