package tasktimer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	engine := New[int](400 * time.Millisecond)
	if engine == nil {
		t.Fatal("engine should not be nil")
	}
	if engine.resolution != 400 {
		t.Errorf("expected resolution 400, got %d", engine.resolution)
	}
}

func TestAddAndGet(t *testing.T) {
	engine := New[string](100 * time.Millisecond)

	executeAt := time.Now().Add(500 * time.Millisecond).UnixMilli()
	var added bool
	engine.Execute(func(tx *Tx[string]) {
		added = tx.Add(&Task[string]{
			JobID:     "job1",
			Topic:     "topic1",
			ExecuteAt: executeAt,
			Data:      "data1",
			Callback:  func(task *Task[string]) {},
		})
	})

	if !added {
		t.Error("task should be added successfully")
	}

	engine.Execute(func(tx *Tx[string]) {
		task, exists := tx.Get("job1")
		if !exists {
			t.Fatal("task should exist")
		}
		if task.JobID != "job1" {
			t.Errorf("expected jobID job1, got %s", task.JobID)
		}
		if task.Data != "data1" {
			t.Errorf("expected data data1, got %s", task.Data)
		}
	})
}

func TestDelete(t *testing.T) {
	engine := New[int](100 * time.Millisecond)

	executeAt := time.Now().Add(500 * time.Millisecond).UnixMilli()
	engine.Execute(func(tx *Tx[int]) {
		added := tx.Add(&Task[int]{
			JobID:     "job1",
			Topic:     "topic1",
			ExecuteAt: executeAt,
			Data:      100,
			Callback:  nil,
		})
		if !added {
			t.Error("task should be added")
		}
	})

	var deletedTask *Task[int]
	var exists bool

	engine.Execute(func(tx *Tx[int]) {
		deletedTask, exists = tx.Delete("job1")
	})

	if !exists {
		t.Fatal("task should exist before deletion")
	}
	if deletedTask.JobID != "job1" {
		t.Errorf("expected jobID job1, got %s", deletedTask.JobID)
	}

	engine.Execute(func(tx *Tx[int]) {
		_, exists := tx.Get("job1")
		if exists {
			t.Error("task should not exist after deletion")
		}
	})
}

func TestReplace(t *testing.T) {
	engine := New[int](100 * time.Millisecond)

	executeAt := time.Now().Add(500 * time.Millisecond).UnixMilli()
	engine.Execute(func(tx *Tx[int]) {
		added := tx.Add(&Task[int]{
			JobID:     "job1",
			Topic:     "topic1",
			ExecuteAt: executeAt,
			Data:      100,
			Callback:  nil,
		})
		if !added {
			t.Error("task should be added")
		}
	})

	newExecuteAt := time.Now().Add(1 * time.Second).UnixMilli()
	oldTask, ok := engine.Replace(&Task[int]{
		JobID:     "job1",
		ExecuteAt: newExecuteAt,
		Data:      200,
		Callback:  nil,
	})
	if !ok {
		t.Fatal("replace should succeed")
	}
	if oldTask.Data != 100 {
		t.Errorf("expected old data 100, got %d", oldTask.Data)
	}

	engine.Execute(func(tx *Tx[int]) {
		task, exists := tx.Get("job1")
		if !exists {
			t.Fatal("task should exist after replace")
		}
		if task.Data != 200 {
			t.Errorf("expected new data 200, got %d", task.Data)
		}
	})
}

func TestCancel(t *testing.T) {
	engine := New[int](100 * time.Millisecond)

	executeAt := time.Now().Add(500 * time.Millisecond).UnixMilli()
	engine.Execute(func(tx *Tx[int]) {
		added := tx.Add(&Task[int]{
			JobID:     "job1",
			Topic:     "topic1",
			ExecuteAt: executeAt,
			Data:      100,
			Callback:  nil,
		})
		if !added {
			t.Error("task should be added")
		}
	})

	task, ok := engine.Cancel("job1")
	if !ok {
		t.Fatal("cancel should succeed")
	}
	if task.JobID != "job1" {
		t.Errorf("expected jobID job1, got %s", task.JobID)
	}

	engine.Execute(func(tx *Tx[int]) {
		_, exists := tx.Get("job1")
		if exists {
			t.Error("task should not exist after cancel")
		}
	})
}

func TestCancelTopic(t *testing.T) {
	engine := New[int](100 * time.Millisecond)

	executeAt := time.Now().Add(500 * time.Millisecond).UnixMilli()
	engine.Execute(func(tx *Tx[int]) {
		tx.Add(&Task[int]{
			JobID:     "job1",
			Topic:     "topic1",
			ExecuteAt: executeAt,
			Data:      100,
			Callback:  nil,
		})
		tx.Add(&Task[int]{
			JobID:     "job2",
			Topic:     "topic1",
			ExecuteAt: executeAt,
			Data:      200,
			Callback:  nil,
		})
		tx.Add(&Task[int]{
			JobID:     "job3",
			Topic:     "topic2",
			ExecuteAt: executeAt,
			Data:      300,
			Callback:  nil,
		})
	})

	count := engine.CancelTopic("topic1")
	if count != 2 {
		t.Errorf("expected 2 cancelled, got %d", count)
	}

	engine.Execute(func(tx *Tx[int]) {
		if _, exists := tx.Get("job1"); exists {
			t.Error("job1 should be cancelled")
		}
		if _, exists := tx.Get("job2"); exists {
			t.Error("job2 should be cancelled")
		}
		if _, exists := tx.Get("job3"); !exists {
			t.Error("job3 should still exist")
		}
	})
}

func TestTick(t *testing.T) {
	engine := New[int](50 * time.Millisecond)
	var callbackCount int32

	executeAt1 := time.Now().Add(100 * time.Millisecond).UnixMilli()
	executeAt2 := time.Now().Add(200 * time.Millisecond).UnixMilli()
	engine.Execute(func(tx *Tx[int]) {
		tx.Add(&Task[int]{
			JobID:     "job1",
			Topic:     "topic1",
			ExecuteAt: executeAt1,
			Data:      100,
			Callback: func(task *Task[int]) {
				atomic.AddInt32(&callbackCount, 1)
			},
		})
		tx.Add(&Task[int]{
			JobID:     "job2",
			Topic:     "topic1",
			ExecuteAt: executeAt2,
			Data:      200,
			Callback: func(task *Task[int]) {
				atomic.AddInt32(&callbackCount, 1)
			},
		})
	})

	now := time.Now().UnixMilli()
	slotKey := (now / engine.resolution) * engine.resolution

	engine.mu.RLock()
	_, exists := engine.slots[slotKey]
	engine.mu.RUnlock()

	if exists {
		engine.Tick()
		time.Sleep(100 * time.Millisecond)

		count := atomic.LoadInt32(&callbackCount)
		if count == 0 {
			t.Error("at least some callbacks should have been executed")
		}
	}
}

func TestStartStop(t *testing.T) {
	engine := New[int](50 * time.Millisecond)
	var callbackCount int32

	engine.Start()

	executeAt := time.Now().Add(100 * time.Millisecond).UnixMilli()
	engine.Execute(func(tx *Tx[int]) {
		tx.Add(&Task[int]{
			JobID:     "job1",
			Topic:     "topic1",
			ExecuteAt: executeAt,
			Data:      100,
			Callback: func(task *Task[int]) {
				atomic.AddInt32(&callbackCount, 1)
			},
		})
	})

	time.Sleep(200 * time.Millisecond)

	engine.Stop()

	count := atomic.LoadInt32(&callbackCount)
	if count == 0 {
		t.Error("callback should have been executed")
	}
}

func TestAddDueTaskAfterStopDispatchOnRestart(t *testing.T) {
	engine := New[int](100 * time.Millisecond)
	engine.Start()

	// 先运行一段时间，确保已进入稳定 Tick 阶段
	time.Sleep(150 * time.Millisecond)
	engine.Stop()

	var callbackCount int32
	engine.Add(&Task[int]{
		JobID:     "due-after-stop",
		ExecuteAt: time.Now().Add(-10 * time.Millisecond).UnixMilli(),
		Data:      1,
		Callback: func(task *Task[int]) {
			atomic.AddInt32(&callbackCount, 1)
		},
	})

	// 停止状态下不应派发新回调
	time.Sleep(120 * time.Millisecond)
	if atomic.LoadInt32(&callbackCount) != 0 {
		t.Fatal("task should not be dispatched while engine is stopped")
	}

	engine.Start()
	defer engine.Stop()

	// 重启后应在后续 Tick 中被执行
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&callbackCount) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if atomic.LoadInt32(&callbackCount) == 0 {
		t.Fatal("task added during stop should execute after restart")
	}
}

func TestStopAndWaitSuccess(t *testing.T) {
	engine := New[int](50 * time.Millisecond)
	engine.Start()

	started := make(chan struct{}, 1)
	finished := make(chan struct{}, 1)

	executeAt := time.Now().Add(50 * time.Millisecond).UnixMilli()
	engine.Add(&Task[int]{
		JobID:     "wait-success",
		ExecuteAt: executeAt,
		Data:      1,
		Callback: func(task *Task[int]) {
			started <- struct{}{}
			time.Sleep(150 * time.Millisecond)
			finished <- struct{}{}
		},
	})

	select {
	case <-started:
	case <-time.After(1 * time.Second):
		t.Fatal("callback should start before StopAndWait")
	}

	start := time.Now()
	ok := engine.StopAndWait(1 * time.Second)
	elapsed := time.Since(start)

	if !ok {
		t.Fatal("StopAndWait should return true when callback completes in timeout")
	}

	select {
	case <-finished:
	default:
		t.Fatal("callback should be finished when StopAndWait returns true")
	}

	if elapsed < 120*time.Millisecond {
		t.Errorf("StopAndWait should wait for callback completion, elapsed=%v", elapsed)
	}
}

func TestStopAndWaitTimeout(t *testing.T) {
	engine := New[int](50 * time.Millisecond)
	engine.Start()

	started := make(chan struct{}, 1)
	var finished int32

	executeAt := time.Now().Add(50 * time.Millisecond).UnixMilli()
	engine.Add(&Task[int]{
		JobID:     "wait-timeout",
		ExecuteAt: executeAt,
		Data:      1,
		Callback: func(task *Task[int]) {
			started <- struct{}{}
			time.Sleep(300 * time.Millisecond)
			atomic.StoreInt32(&finished, 1)
		},
	})

	select {
	case <-started:
	case <-time.After(1 * time.Second):
		t.Fatal("callback should start before StopAndWait")
	}

	start := time.Now()
	ok := engine.StopAndWait(50 * time.Millisecond)
	elapsed := time.Since(start)

	if ok {
		t.Fatal("StopAndWait should return false when timeout reached")
	}

	if elapsed < 40*time.Millisecond {
		t.Errorf("StopAndWait should wait close to timeout before returning, elapsed=%v", elapsed)
	}

	if atomic.LoadInt32(&finished) != 0 {
		t.Fatal("callback should not have finished when StopAndWait timed out")
	}

	time.Sleep(350 * time.Millisecond)
	if atomic.LoadInt32(&finished) == 0 {
		t.Fatal("callback should finish eventually after timeout return")
	}
}

func TestMultipleTicks(t *testing.T) {
	engine := New[int](50 * time.Millisecond)
	var callbackCount int32

	executeAt1 := time.Now().Add(50 * time.Millisecond).UnixMilli()
	executeAt2 := time.Now().Add(150 * time.Millisecond).UnixMilli()
	engine.Execute(func(tx *Tx[int]) {
		tx.Add(&Task[int]{
			JobID:     "job1",
			Topic:     "topic1",
			ExecuteAt: executeAt1,
			Data:      100,
			Callback: func(task *Task[int]) {
				atomic.AddInt32(&callbackCount, 1)
			},
		})
		tx.Add(&Task[int]{
			JobID:     "job2",
			Topic:     "topic1",
			ExecuteAt: executeAt2,
			Data:      200,
			Callback: func(task *Task[int]) {
				atomic.AddInt32(&callbackCount, 1)
			},
		})
	})

	time.Sleep(50 * time.Millisecond)
	engine.Tick()

	time.Sleep(100 * time.Millisecond)
	engine.Tick()

	time.Sleep(100 * time.Millisecond)

	count := atomic.LoadInt32(&callbackCount)
	if count != 2 {
		t.Errorf("expected 2 callbacks, got %d", count)
	}
}

func TestCallbackPanic(t *testing.T) {
	engine := New[int](50 * time.Millisecond)

	executeAt := time.Now().Add(50 * time.Millisecond).UnixMilli()
	engine.Execute(func(tx *Tx[int]) {
		tx.Add(&Task[int]{
			JobID:     "job1",
			Topic:     "topic1",
			ExecuteAt: executeAt,
			Data:      100,
			Callback: func(task *Task[int]) {
				panic("test panic")
			},
		})
	})

	defer func() {
		if r := recover(); r != nil {
			t.Error("panic should be recovered in callback")
		}
	}()

	engine.Tick()
	time.Sleep(100 * time.Millisecond)
}

func TestEmptySlotCleanup(t *testing.T) {
	engine := New[int](50 * time.Millisecond)

	executeAt := time.Now().Add(100 * time.Millisecond).UnixMilli()
	engine.Execute(func(tx *Tx[int]) {
		tx.Add(&Task[int]{
			JobID:     "job1",
			Topic:     "topic1",
			ExecuteAt: executeAt,
			Data:      100,
			Callback:  nil,
		})
	})

	engine.Execute(func(tx *Tx[int]) {
		tx.Delete("job1")
	})

	engine.mu.RLock()
	if len(engine.slots) != 0 {
		t.Errorf("slots should be empty after deletion, got %d slots", len(engine.slots))
	}
	engine.mu.RUnlock()
}

func TestTopicCallback(t *testing.T) {
	engine := New[int](50 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	var callbackCount int32
	var receivedTask *Task[int]
	var mu sync.Mutex

	// 注册主题回调
	engine.RegisterTopic("topic1", func(task *Task[int]) {
		atomic.AddInt32(&callbackCount, 1)
		mu.Lock()
		receivedTask = task
		mu.Unlock()
	})

	// 添加任务时不指定回调，应该使用主题回调
	executeAt := time.Now().Add(100 * time.Millisecond).UnixMilli()
	engine.Execute(func(tx *Tx[int]) {
		tx.Add(&Task[int]{
			JobID:     "job1",
			Topic:     "topic1",
			ExecuteAt: executeAt,
			Data:      100,
			Callback:  nil,
		})
	})

	// 等待任务执行
	time.Sleep(200 * time.Millisecond)

	// 验证回调被执行
	count := atomic.LoadInt32(&callbackCount)
	if count != 1 {
		t.Errorf("expected 1 callback, got %d", count)
	}

	// 验证回调接收到了完整的 Task 信息
	mu.Lock()
	defer mu.Unlock()

	if receivedTask == nil {
		t.Fatal("received task should not be nil")
	}
	if receivedTask.JobID != "job1" {
		t.Errorf("expected jobID job1, got %s", receivedTask.JobID)
	}
	if receivedTask.Data != 100 {
		t.Errorf("expected data 100, got %d", receivedTask.Data)
	}
	if receivedTask.Topic != "topic1" {
		t.Errorf("expected topic topic1, got %s", receivedTask.Topic)
	}
	if receivedTask.ExecuteAt == 0 {
		t.Error("ExecuteAt should be set")
	}
}

func TestTopicCallbackOverride(t *testing.T) {
	engine := New[int](50 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	var topicCallbackCount int32
	var taskCallbackCount int32

	// 注册主题回调
	engine.RegisterTopic("topic1", func(task *Task[int]) {
		atomic.AddInt32(&topicCallbackCount, 1)
	})

	// 添加任务时指定回调，应该使用任务指定的回调
	executeAt := time.Now().Add(100 * time.Millisecond).UnixMilli()
	engine.Execute(func(tx *Tx[int]) {
		tx.Add(&Task[int]{
			JobID:     "job1",
			Topic:     "topic1",
			ExecuteAt: executeAt,
			Data:      100,
			Callback: func(task *Task[int]) {
				atomic.AddInt32(&taskCallbackCount, 1)
			},
		})
	})

	// 等待任务执行
	time.Sleep(200 * time.Millisecond)

	// 验证任务指定的回调被执行，主题回调未被执行
	if atomic.LoadInt32(&topicCallbackCount) != 0 {
		t.Error("topic callback should not be called when task has its own callback")
	}
	if atomic.LoadInt32(&taskCallbackCount) != 1 {
		t.Errorf("expected 1 task callback, got %d", atomic.LoadInt32(&taskCallbackCount))
	}
}

func TestAddOrReplace(t *testing.T) {
	engine := New[int](100 * time.Millisecond)

	// 测试添加新任务
	executeAt1 := time.Now().Add(500 * time.Millisecond).UnixMilli()
	oldTask, replaced := engine.AddOrReplace(&Task[int]{
		JobID:     "job1",
		Topic:     "topic1",
		ExecuteAt: executeAt1,
		Data:      100,
		Callback:  nil,
	})

	if replaced {
		t.Error("should not replace when adding new task")
	}
	if oldTask != nil {
		t.Error("old task should be nil when adding new task")
	}

	// 验证任务存在
	task, exists := engine.Get("job1")
	if !exists {
		t.Fatal("task should exist")
	}
	if task.Data != 100 {
		t.Errorf("expected data 100, got %d", task.Data)
	}

	// 测试替换已存在的任务
	executeAt2 := time.Now().Add(1 * time.Second).UnixMilli()
	oldTask, replaced = engine.AddOrReplace(&Task[int]{
		JobID:     "job1",
		Topic:     "topic1",
		ExecuteAt: executeAt2,
		Data:      200,
		Callback:  nil,
	})

	if !replaced {
		t.Error("should replace when task exists")
	}
	if oldTask == nil {
		t.Fatal("old task should not be nil when replacing")
	}
	if oldTask.Data != 100 {
		t.Errorf("expected old data 100, got %d", oldTask.Data)
	}

	// 验证任务被更新
	task, exists = engine.Get("job1")
	if !exists {
		t.Fatal("task should still exist")
	}
	if task.Data != 200 {
		t.Errorf("expected new data 200, got %d", task.Data)
	}
}

func TestAddDuplicate(t *testing.T) {
	engine := New[int](100 * time.Millisecond)

	executeAt1 := time.Now().Add(500 * time.Millisecond).UnixMilli()
	added := engine.Add(&Task[int]{
		JobID:     "job1",
		Topic:     "topic1",
		ExecuteAt: executeAt1,
		Data:      100,
		Callback:  nil,
	})

	if !added {
		t.Error("first add should succeed")
	}

	// 尝试添加相同 JobID 的任务
	executeAt2 := time.Now().Add(1 * time.Second).UnixMilli()
	added = engine.Add(&Task[int]{
		JobID:     "job1",
		Topic:     "topic2",
		ExecuteAt: executeAt2,
		Data:      200,
		Callback:  nil,
	})

	if added {
		t.Error("second add should fail for duplicate jobID")
	}

	// 验证原任务未被覆盖
	task, exists := engine.Get("job1")
	if !exists {
		t.Fatal("task should still exist")
	}
	if task.Data != 100 {
		t.Errorf("expected original data 100, got %d", task.Data)
	}
	if task.Topic != "topic1" {
		t.Errorf("expected original topic topic1, got %s", task.Topic)
	}
}
