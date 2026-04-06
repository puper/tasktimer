package tasktimer

import (
	"testing"
	"time"
)

// TestStopRestartPanic 复现 CRITICAL-001: Stop() 后重新 Start() 导致 panic
// 这个测试验证了当引擎被停止后再次启动时，第二次 Stop() 会尝试关闭已关闭的 channel
func TestStopRestartPanic(t *testing.T) {
	engine := New[string](400 * time.Millisecond)

	// 第一次启动和停止
	engine.Start()
	time.Sleep(100 * time.Millisecond)
	engine.Stop()

	// 等待引擎完全停止
	time.Sleep(100 * time.Millisecond)

	// 重新启动引擎
	engine.Start()
	time.Sleep(100 * time.Millisecond)

	// 第二次停止 - 这里应该 panic: close of closed channel
	defer func() {
		if r := recover(); r != nil {
			t.Logf("捕获到预期的 panic: %v", r)
			t.Log("这验证了 CRITICAL-001 漏洞：Stop() 后重新 Start() 导致 panic")
		}
	}()

	engine.Stop()

	t.Log("如果没有 panic，说明 Bug 已修复")
}

// TestStopRestartMultipleTimes 测试多次启动和停止
func TestStopRestartMultipleTimes(t *testing.T) {
	engine := New[string](400 * time.Millisecond)

	for i := 0; i < 5; i++ {
		t.Logf("第 %d 次启动引擎", i+1)
		engine.Start()
		time.Sleep(50 * time.Millisecond)

		t.Logf("第 %d 次停止引擎", i+1)
		engine.Stop()
		time.Sleep(50 * time.Millisecond)
	}

	t.Log("如果所有启动和停止都成功，说明 Bug 已修复")
}

// TestStopRestartWithTasks 测试在重新启动时任务是否正常工作
func TestStopRestartWithTasks(t *testing.T) {
	engine := New[string](400 * time.Millisecond)
	executed := make(chan string, 10)

	// 第一次启动并添加任务
	engine.Start()
	engine.RegisterTopic("test", func(task *Task[string]) {
		executed <- task.JobID
	})

	engine.Add(&Task[string]{
		JobID:     "task1",
		Topic:     "test",
		ExecuteAt: time.Now().Add(500 * time.Millisecond).UnixMilli(),
		Data:      "data1",
	})

	time.Sleep(600 * time.Millisecond)

	select {
	case jobID := <-executed:
		t.Logf("第一次启动期间执行了任务: %s", jobID)
	case <-time.After(1 * time.Second):
		t.Error("第一次启动期间任务未执行")
	}

	// 停止引擎
	engine.Stop()
	time.Sleep(100 * time.Millisecond)

	// 重新启动引擎
	defer func() {
		if r := recover(); r != nil {
			t.Logf("捕获到 panic: %v", r)
			t.Error("重新启动失败，CRITICAL-001 漏洞仍然存在")
		}
	}()

	engine.Start()

	// 添加新任务
	engine.Add(&Task[string]{
		JobID:     "task2",
		Topic:     "test",
		ExecuteAt: time.Now().Add(500 * time.Millisecond).UnixMilli(),
		Data:      "data2",
	})

	time.Sleep(600 * time.Millisecond)

	select {
	case jobID := <-executed:
		t.Logf("重新启动后执行了任务: %s", jobID)
	case <-time.After(1 * time.Second):
		t.Error("重新启动后任务未执行")
	}

	engine.Stop()
}
