# AGENTS.md

本项目默认使用中文进行注释、回复和文档编写。

## 项目概览

TaskTimer 是一个 Go 语言实现的延迟任务调度引擎，支持基于时间槽的延迟任务执行、任务取消和主题批量管理。

核心特性：
- 泛型支持，可存储任意类型数据
- 基于时间槽的高效调度
- 支持任务主题分类和批量取消
- 线程安全的事务操作
- 回调函数 panic 恢复

## 从哪里开始

- **入口点**: `tasktimer.go` - 核心引擎实现
- **使用示例**: `example/main.go` - 基本用法演示
- **测试**: `tasktimer_test.go` - 功能测试和用法参考

## 仓库结构

```
.
├── tasktimer.go          # 核心引擎实现
├── tasktimer_test.go     # 单元测试
├── example/              # 使用示例
│   └── main.go
├── go.mod                # Go 模块定义
└── docs/                 # 项目文档
```

## 常用命令

```bash
# 运行测试
go test -v

# 运行示例
go run example/main.go

# 构建验证
go build
```

## 核心类型

- `Engine[T]` - 任务调度引擎
- `Task[T]` - 任务定义
- `Tx[T]` - 事务对象，用于原子操作

## 主要操作

- `New[T](resolution)` - 创建引擎，指定时间精度
- `Start()` / `Stop()` - 启动/停止引擎
- `Execute(fn)` - 在事务中执行操作
- `Add()` / `Get()` / `Delete()` - 任务 CRUD
- `Cancel()` / `CancelTopic()` - 取消任务
- `Replace()` - 替换任务

## 变更规范

1. 保持向后兼容性
2. 新增功能需添加测试
3. 使用 `Execute()` 进行事务操作
4. 回调函数必须处理 panic

## 验证清单

- [ ] 所有测试通过：`go test -v`
- [ ] 代码格式化：`go fmt`
- [ ] 示例可运行：`go run example/main.go`

## 文档索引

详细文档位于 `docs/` 目录：
- `docs/index.md` - 文档导航
- `docs/design/` - 设计文档
- `docs/workflows/` - 开发流程
