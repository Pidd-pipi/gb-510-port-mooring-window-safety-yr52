# 验收记录

- 日期：2026-08-22
- 静态检查：Go 1.22 下 `go test ./...`、`go test -race ./...`、`go vet ./...`、`go build ./...` 通过；Vue 类型检查和 Vite 生产构建通过；`docker compose config --quiet` 通过。
- 容器启动：PostgreSQL、Redis、backend、frontend 从空数据卷启动成功并达到 healthy，`GET /healthz` 返回 200，后端运行日志未见异常。
- API 流程：管理员登录、概览、4 个实体列表、创建靠泊任务、合法状态迁移、会话、脱敏运行配置、审计列表及审计汇总均通过。
- RBAC 与双人确认：viewer 写操作返回 403；operator 首次提交后许可保持 `pending`；提交人自审返回 422；不同账号的 reviewer 完成放行。审计保存窗口版本、操作者和请求 ID。
- 内置 Browser：验证船舶靠泊、系泊方案、风浪窗口、安全许可、审计记录 5 个页面；`RiskBadge` 与跨页 `ClearancePanel` 正常展示；实际以 admin 复核 operator 已提交的 `SC-001`，状态刷新为 `cleared`，审计回显窗口 v1 与请求 ID；控制台 0 error / 0 warning，桌面截图未见遮挡或错位。
- 规模：3095 行 Go 功能代码，38 个非测试 `.go` 文件。
- 清理：验收完成后执行 `docker compose down -v --remove-orphans`，清除本项目容器和数据卷。

## 泊位时段占用闭环验收（2026-09-22）

- 静态检查：Go 1.22（arm64）下 `go test ./...`、`go test -race -count=5`（占用与并发用例）、`go vet ./...`、`go build ./...` 通过；`gofmt -l` 无输出；Vue `tsc --noEmit` 与 Vite 生产构建通过。
- 批准前置条件：缺少泊位/起止时间/窗口返回 422 且方案保持原状态、占用数为 0；关联窗口为 restricted 或不存在返回 422；结束早于开始返回 422；viewer 调用批准返回 403。
- 占用获取：safe 窗口 + 空闲时段批准成功（200），方案回读 `berthCode/berthStartAt/berthEndAt/windowCode/currentOccupancyId`，`GET /api/berth-occupancies` 出现 active 记录。
- 冲突判定：重叠时段批准返回 409 `berth_occupied` 且失败方案无占用、版本不变；端点相接（end == start）判定空闲；冲突预览接口分别返回 free=true 与冲突方案明细。
- 并发：HTTP 层 3 个并行请求批准同一泊位同一时段，恰好 1 个 200、2 个 409，active 占用与 approved 方案均为 1（Go 层 8 并发单测 + 5 轮 race 复跑一致）。
- 释放闭环：撤回与替代均在事务内将占用置为 released（释放人/时间/原因回读），方案回到 review / superseded 且占用指针清零；释放后原时段可重新批准；`occupancy_acquire`、`occupancy_release`、`occupancy_release_supersede` 审计均带 actor 与 requestId；已批准方案禁止普通编辑与直接删除。
- 回归：许可双人确认不受影响——提交人自审 422，不同账号 reviewer 放行 200，审计窗口版本保留。
