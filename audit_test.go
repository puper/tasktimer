package tasktimer

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestTaskLossAfterTick 验证任务丢失 Bug 修复
// 场景：在 Tick 删除时间槽后，添加任务到已过期的槽中
// 预期：任务应该立即执行，而不是永久丢失
func TestTaskLossAfterTick(t *testing.T) {
	engine := New[int](400 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	// 等待第一个 Tick 完成
	time.Sleep(500 * time.Millisecond)

	// 添加一个明显已过期的任务（ExecuteAt 在 1 秒前）
	executeAt := time.Now().Add(-1 * time.Second).UnixMilli()
	var executed int32

	added := engine.Add(&Task[int]{
		JobID:     "delayed-task",
		Topic:     "test",
		ExecuteAt: executeAt,
		Data:      100,
		Callback: func(task *Task[int]) {
			atomic.AddInt32(&executed, 1)
		},
	})

	if !added {
		t.Error("任务添加失败")
	}

	// 等待任务执行
	time.Sleep(200 * time.Millisecond)

	count := atomic.LoadInt32(&executed)
	if count == 0 {
		t.Errorf("任务丢失 Bug 未修复：任务添加到已过期的时间槽，但未被执行")
		t.Errorf("ExecuteAt: %d, 当前时间: %d", executeAt, time.Now().UnixMilli())
	} else {
		t.Logf("Bug 已修复：过期任务立即执行，执行次数: %d", count)
	}
}

// TestTaskLossRaceCondition 验证竞态条件下的任务丢失修复
// 场景：Tick 执行期间，并发添加任务到当前时间槽
func TestTaskLossRaceCondition(t *testing.T) {
	engine := New[int](100 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	var executedCount int32
	var addedCount int32

	// 并发添加任务
	for i := 0; i < 100; i++ {
		go func(id int) {
			// 在不同时间点添加任务
			time.Sleep(time.Duration(id%10) * time.Millisecond)

			executeAt := time.Now().Add(time.Duration(id%5) * 100 * time.Millisecond).UnixMilli()
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
	time.Sleep(2 * time.Second)

	added := atomic.LoadInt32(&addedCount)
	executed := atomic.LoadInt32(&executedCount)

	t.Logf("添加任务数: %d, 执行任务数: %d", added, executed)

	if executed < added {
		lost := added - executed
		t.Errorf("检测到任务丢失：丢失 %d 个任务", lost)
	}
}

// TestPanicInformationLoss 验证 Panic 信息记录功能
// 场景：任务回调中发生 panic，通过错误处理器记录
func TestPanicInformationLoss(t *testing.T) {
	engine := New[int](50 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	var panicCaptured int32
	var panicValue interface{}
	var panickedTask *Task[int]
	var mu sync.Mutex

	// 设置错误处理器
	engine.SetErrorHandler(func(task *Task[int], err interface{}) {
		mu.Lock()
		defer mu.Unlock()
		atomic.StoreInt32(&panicCaptured, 1)
		panicValue = err
		panickedTask = task
	})

	// 添加任务，ExecuteAt 在未来
	executeAt := time.Now().Add(100 * time.Millisecond).UnixMilli()
	engine.Add(&Task[int]{
		JobID:     "panic-task",
		ExecuteAt: executeAt,
		Data:      100,
		Callback: func(task *Task[int]) {
			panic("critical error in task")
		},
	})

	// 等待任务执行
	time.Sleep(300 * time.Millisecond)

	// 验证错误处理器被调用
	mu.Lock()
	defer mu.Unlock()

	if atomic.LoadInt32(&panicCaptured) == 0 {
		t.Error("Panic 未被错误处理器捕获")
	}

	if panicValue == nil {
		t.Error("Panic 值未保存")
	} else if panicValue != "critical error in task" {
		t.Errorf("Panic 值不正确: %v", panicValue)
	}

	if panickedTask == nil {
		t.Error("任务信息未保存")
	} else if panickedTask.JobID != "panic-task" {
		t.Errorf("任务 JobID 不正确: %s", panickedTask.JobID)
	} else {
		t.Log("Bug 已修复：Panic 信息通过错误处理器正确记录")
	}
}

// TestGoroutineLeak 验证 Goroutine 泄漏修复
// 场景：创建大量阻塞任务，验证工作池限制
func TestGoroutineLeak(t *testing.T) {
	// 创建工作池大小为 5 的引擎
	engine := New[int](50*time.Millisecond, 5)

	initialGoroutines := runtime.NumGoroutine()
	t.Logf("初始 goroutine 数量: %d", initialGoroutines)

	// 创建 10 个阻塞任务，但工作池只有 5 个槽位
	for i := 0; i < 10; i++ {
		executeAt := time.Now().Add(50 * time.Millisecond).UnixMilli()
		engine.Add(&Task[int]{
			JobID:     string(rune('A' + i)),
			ExecuteAt: executeAt,
			Data:      i,
			Callback: func(task *Task[int]) {
				// 阻塞 500 毫秒
				time.Sleep(500 * time.Millisecond)
			},
		})
	}

	engine.Tick()
	time.Sleep(100 * time.Millisecond)

	afterTickGoroutines := runtime.NumGoroutine()
	t.Logf("Tick 后 goroutine 数量: %d", afterTickGoroutines)

	// 验证并发限制：最多只有 5 个任务在并发执行
	// 初始 goroutine + 5 个工作池 goroutine + 少量系统 goroutine
	leaked := afterTickGoroutines - initialGoroutines
	t.Logf("新增 goroutine 数量: %d", leaked)

	// 由于工作池限制，新增 goroutine 应该 <= 5
	if leaked > 7 { // 允许一些误差
		t.Errorf("工作池限制失效：创建了 %d 个 goroutine，预期最多 5 个", leaked)
	} else {
		t.Log("Bug 已修复：工作池成功限制并发 goroutine 数量")
	}

	// 等待任务完成
	time.Sleep(3 * time.Second)

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("最终 goroutine 数量: %d", finalGoroutines)

	if finalGoroutines > initialGoroutines+2 {
		t.Errorf("存在 goroutine 泄漏：最终数量 %d > 初始数量 %d", finalGoroutines, initialGoroutines)
	}
}

// TestGoroutineLeakUnderLoad 验证高负载下的 Goroutine 泄漏修复
// 场景：持续添加任务，模拟生产环境
func TestGoroutineLeakUnderLoad(t *testing.T) {
	engine := New[int](50*time.Millisecond, 10)
	engine.Start()
	defer engine.Stop()

	initialGoroutines := runtime.NumGoroutine()
	t.Logf("初始 goroutine 数量: %d", initialGoroutines)

	// 模拟高负载：每 10ms 添加一个任务，持续 1 秒
	for i := 0; i < 100; i++ {
		go func(id int) {
			executeAt := time.Now().Add(100 * time.Millisecond).UnixMilli()
			engine.Add(&Task[int]{
				JobID:     string(rune('A' + id)),
				ExecuteAt: executeAt,
				Data:      id,
				Callback: func(task *Task[int]) {
					// 模拟一些任务阻塞
					if id%10 == 0 {
						time.Sleep(500 * time.Millisecond)
					}
				},
			})
		}(i)
		time.Sleep(10 * time.Millisecond)
	}

	// 等待任务执行
	time.Sleep(2 * time.Second)

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("最终 goroutine 数量: %d", finalGoroutines)

	leaked := finalGoroutines - initialGoroutines
	if leaked > 15 { // 工作池 10 + 一些误差
		t.Errorf("高负载下存在 goroutine 泄漏：泄漏 %d 个 goroutine", leaked)
	} else {
		t.Log("高负载下工作池正常工作")
	}
}

// TestStopDoesNotWaitForCallbacks 验证 Stop 不等待回调完成
// 场景：Stop() 立即返回，不等待正在执行的回调
func TestStopDoesNotWaitForCallbacks(t *testing.T) {
	engine := New[int](50 * time.Millisecond)
	engine.Start()

	var callbackCompleted int32

	// 添加一个长时间运行的任务
	executeAt := time.Now().Add(50 * time.Millisecond).UnixMilli()
	engine.Add(&Task[int]{
		JobID:     "long-task",
		ExecuteAt: executeAt,
		Data:      100,
		Callback: func(task *Task[int]) {
			time.Sleep(1 * time.Second)
			atomic.StoreInt32(&callbackCompleted, 1)
		},
	})

	// 等待任务开始执行
	time.Sleep(100 * time.Millisecond)

	// 立即停止引擎
	start := time.Now()
	engine.Stop()
	elapsed := time.Since(start)

	t.Logf("Stop() 耗时: %v", elapsed)

	if elapsed < 100*time.Millisecond {
		t.Log("Stop() 立即返回，未等待回调完成")
	}

	time.Sleep(2 * time.Second)

	if atomic.LoadInt32(&callbackCompleted) == 0 {
		t.Error("回调未完成就被中断，可能导致数据丢失")
	}
}
