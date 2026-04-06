package tasktimer

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestTickRaceCondition 验证 Tick 和 Add 的竞态条件
// 场景：Tick 处理完当前槽后，Add 添加任务到当前槽
func TestTickRaceCondition(t *testing.T) {
	engine := New[int](100 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	var executedCount int32

	// 等待第一个 Tick 完成
	time.Sleep(150 * time.Millisecond)

	// 此时 Tick 已经处理过当前时间槽
	// 添加一个任务到当前时间槽（应该立即执行，而不是丢失）
	now := time.Now().UnixMilli()
	currentSlotKey := (now / engine.resolution) * engine.resolution

	// 添加任务到当前时间槽
	// 由于 Tick 已经处理过这个槽，任务应该立即执行
	executeAt := now + 50 // 50ms 后执行（仍在当前槽内）
	engine.Add(&Task[int]{
		JobID:     "task-after-tick",
		ExecuteAt: executeAt,
		Data:      100,
		Callback: func(task *Task[int]) {
			atomic.AddInt32(&executedCount, 1)
		},
	})

	// 等待任务执行
	time.Sleep(200 * time.Millisecond)

	count := atomic.LoadInt32(&executedCount)
	if count == 0 {
		t.Errorf("任务丢失！Tick 处理后添加到当前槽的任务未被执行")
		t.Errorf("当前槽: %d, lastProcessedSlot: %d", currentSlotKey, engine.lastProcessedSlot)
	} else {
		t.Logf("Bug 已修复：任务立即执行，执行次数: %d", count)
	}
}

// TestTickAndAddConcurrent 验证并发场景下的正确性
// 场景：Tick 和 Add 并发执行
func TestTickAndAddConcurrent(t *testing.T) {
	engine := New[int](50 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	var executedCount int32
	var addedCount int32

	// 并发添加任务，同时 Tick 在运行
	for i := 0; i < 100; i++ {
		go func(id int) {
			// 随机延迟添加
			time.Sleep(time.Duration(id%20) * time.Millisecond)

			// 添加任务到当前或未来的时间槽
			now := time.Now().UnixMilli()
			executeAt := now + int64((id%3)*50) // 0, 50, 100ms 后

			added := engine.Add(&Task[int]{
				JobID:     string(rune('A' + id)),
				ExecuteAt: executeAt,
				Data:      id,
				Callback: func(task *Task[int]) {
					atomic.AddInt32(&executedCount, 1)
				},
			})

			if added {
				atomic.AddInt32(&addedCount, 1)
			}
		}(i)
	}

	// 等待所有任务执行完成
	time.Sleep(500 * time.Millisecond)

	added := atomic.LoadInt32(&addedCount)
	executed := atomic.LoadInt32(&executedCount)

	t.Logf("添加任务数: %d, 执行任务数: %d", added, executed)

	// 所有添加的任务都应该被执行
	if executed < added {
		lost := added - executed
		t.Errorf("检测到任务丢失：丢失 %d 个任务", lost)
	}
}

// TestAddToProcessedSlot 验证添加到已处理槽的任务立即执行
func TestAddToProcessedSlot(t *testing.T) {
	engine := New[int](100 * time.Millisecond)

	var executed int32

	// 先执行一次 Tick，设置 lastProcessedSlot
	time.Sleep(50 * time.Millisecond)
	engine.Tick()

	// 获取 lastProcessedSlot
	engine.mu.RLock()
	lastProcessed := engine.lastProcessedSlot
	engine.mu.RUnlock()

	// 添加任务到已处理的槽（与 lastProcessedSlot 相同的槽）
	// 使用槽的起始时间，确保在同一个槽内
	executeAt := lastProcessed + 50
	engine.Add(&Task[int]{
		JobID:     "task-in-processed-slot",
		ExecuteAt: executeAt,
		Data:      100,
		Callback: func(task *Task[int]) {
			atomic.AddInt32(&executed, 1)
		},
	})

	// 等待任务执行
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&executed) == 0 {
		t.Error("添加到已处理槽的任务应该立即执行")
	} else {
		t.Log("Bug 已修复：添加到已处理槽的任务立即执行")
	}
}
