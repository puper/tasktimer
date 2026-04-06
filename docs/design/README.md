# 设计文档

本目录包含 TaskTimer 的架构设计和核心概念。

## 核心设计

### 时间槽机制

TaskTimer 使用时间槽（Time Slot）进行任务调度：

- 将时间划分为固定大小的槽
- 每个槽存储该时间段内需要执行的任务
- 按槽精度检查和执行任务

**优势**：
- O(1) 插入和查找
- 避免优先队列的复杂度
- 适合延迟任务场景

### 事务模型

所有任务操作通过 `Execute()` 方法包装：

```go
engine.Execute(func(tx *Tx[T]) {
    tx.Add(...)
    tx.Delete(...)
})
```

**原因**：
- 保证原子性
- 简化并发控制
- 避免锁泄漏

### 索引结构

使用三个映射索引任务：

1. `slots` - 时间槽到任务列表
2. `jobToSlot` - 任务 ID 到时间槽
3. `topicIndex` - 主题到任务 ID 集合

**优势**：
- 快速查找任务
- 支持按主题批量操作
- 空间效率高

## 架构图

```
Engine[T]
├── slots: map[int64]map[string]*Task[T]
├── jobToSlot: map[string]int64
└── topicIndex: map[string]map[string]struct{}
```

## 并发模型

- 使用 `sync.RWMutex` 保护所有状态
- 回调在独立 goroutine 中执行
- Panic 在回调中恢复，不影响引擎

## 性能特征

- 时间复杂度：O(1) 插入、查找、删除
- 空间复杂度：O(n) n 为任务数量
- 精度：由 resolution 参数决定
