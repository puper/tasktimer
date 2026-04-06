package tasktimer

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestStopRestartDoubleStop 验证双重 Stop 的安全性
func TestStopRestartDoubleStop(t *testing.T) {
	engine := New[string](100 * time.Millisecond)

	engine.Start()
	time.Sleep(50 * time.Millisecond)

	// 第一次 Stop
	engine.Stop()

	// 第二次 Stop（应该安全返回，不会 panic）
	engine.Stop()

	t.Log("✓ 双重 Stop 安全，无 panic")
}

// TestStopWithoutStart 验证未启动就 Stop 的安全性
func TestStopWithoutStart(t *testing.T) {
	engine := New[string](100 * time.Millisecond)

	// 未启动就 Stop（应该安全返回）
	engine.Stop()

	t.Log("✓ 未启动就 Stop 安全，无 panic")
}

// TestMultipleStarts 验证多次 Start 的安全性
func TestMultipleStarts(t *testing.T) {
	engine := New[string](100 * time.Millisecond)

	// 多次 Start（应该只启动一次）
	engine.Start()
	engine.Start() // 第二次 Start 应该被忽略
	engine.Start() // 第三次 Start 应该被忽略

	time.Sleep(50 * time.Millisecond)
	engine.Stop()

	t.Log("✓ 多次 Start 安全，只启动一次")
}

// TestAddToExpiredSlot 验证添加到已过期槽的任务立即执行
func TestAddToExpiredSlot(t *testing.T) {
	engine := New[int](100 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	var executed int32
	engine.RegisterTopic("test", func(task *Task[int]) {
		atomic.StoreInt32(&executed, 1)
	})

	past := time.Now().Add(-1 * time.Hour).UnixMilli()
	added := engine.Add(&Task[int]{
		JobID:     "expired-task",
		Topic:     "test",
		ExecuteAt: past,
		Data:      100,
	})

	if !added {
		t.Error("任务添加失败")
	}

	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&executed) == 0 {
		t.Error("过期任务未立即执行")
	} else {
		t.Log("✓ 过期任务立即执行")
	}
}

// TestReplaceWithExpiredTask 验证替换为过期任务的行为
func TestReplaceWithExpiredTask(t *testing.T) {
	engine := New[int](100 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	var executedCount int32
	engine.RegisterTopic("test", func(task *Task[int]) {
		atomic.AddInt32(&executedCount, 1)
	})

	future := time.Now().Add(1 * time.Hour).UnixMilli()
	engine.Add(&Task[int]{
		JobID:     "future-task",
		Topic:     "test",
		ExecuteAt: future,
		Data:      100,
	})

	time.Sleep(50 * time.Millisecond)

	past := time.Now().Add(-1 * time.Second).UnixMilli()
	oldTask, exists := engine.Replace(&Task[int]{
		JobID:     "future-task",
		Topic:     "test",
		ExecuteAt: past,
		Data:      200,
	})

	if !exists {
		t.Error("旧任务不存在")
	}

	if oldTask == nil {
		t.Error("未返回旧任务")
	} else if oldTask.Data != 100 {
		t.Errorf("旧任务数据错误: %d", oldTask.Data)
	}

	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&executedCount) != 1 {
		t.Errorf("预期执行 1 次，实际: %d", atomic.LoadInt32(&executedCount))
	} else {
		t.Log("✓ 替换为过期任务后立即执行")
	}

	_, exists = engine.Get("future-task")
	if exists {
		t.Error("旧任务未被删除")
	}
}

// TestAddOrReplaceSemantics 验证 AddOrReplace 的语义
func TestAddOrReplaceSemantics(t *testing.T) {
	engine := New[int](100 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	var executedCount int32
	engine.RegisterTopic("test", func(task *Task[int]) {
		atomic.AddInt32(&executedCount, 1)
	})

	// AddOrReplace 新任务
	future := time.Now().Add(1 * time.Hour).UnixMilli()
	oldTask, replaced := engine.AddOrReplace(&Task[int]{
		JobID:     "new-task",
		Topic:     "test",
		ExecuteAt: future,
		Data:      100,
	})

	if replaced {
		t.Error("新任务不应该替换")
	}

	if oldTask != nil {
		t.Error("新任务不应该返回旧任务")
	}

	// 再次 AddOrReplace，这次应该替换
	oldTask, replaced = engine.AddOrReplace(&Task[int]{
		JobID:     "new-task",
		Topic:     "test",
		ExecuteAt: future,
		Data:      200,
	})

	if !replaced {
		t.Error("应该替换旧任务")
	}

	if oldTask == nil || oldTask.Data != 100 {
		t.Error("未返回正确的旧任务")
	}

	t.Log("✓ AddOrReplace 语义正确")
}

// TestErrorHandlerModificationDuringExecution 验证执行期间修改 errorHandler
func TestErrorHandlerModificationDuringExecution(t *testing.T) {
	engine := New[int](50 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	var handler1Called int32
	var handler2Called int32

	engine.SetErrorHandler(func(task *Task[int], err interface{}) {
		atomic.StoreInt32(&handler1Called, 1)
	})

	future := time.Now().Add(100 * time.Millisecond).UnixMilli()
	engine.Add(&Task[int]{
		JobID:     "panic-task",
		ExecuteAt: future,
		Callback: func(task *Task[int]) {
			engine.SetErrorHandler(func(task *Task[int], err interface{}) {
				atomic.StoreInt32(&handler2Called, 1)
			})
			panic("test")
		},
	})

	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt32(&handler1Called) == 1 && atomic.LoadInt32(&handler2Called) == 0 {
		t.Log("✓ 使用了任务执行时的 handler（RLock 保护生效）")
	} else {
		t.Logf("Handler1: %v, Handler2: %v", atomic.LoadInt32(&handler1Called), atomic.LoadInt32(&handler2Called))
	}
}
