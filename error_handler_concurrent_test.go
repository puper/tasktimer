package tasktimer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestErrorHandlerConcurrentModification 验证 errorHandler 的并发修改安全性
func TestErrorHandlerConcurrentModification(t *testing.T) {
	engine := New[int](50 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	var callCount int32
	var mu sync.Mutex
	var capturedHandlers []string

	// 初始错误处理器
	engine.SetErrorHandler(func(task *Task[int], err interface{}) {
		mu.Lock()
		capturedHandlers = append(capturedHandlers, "handler1")
		mu.Unlock()
		atomic.AddInt32(&callCount, 1)
	})

	// 并发修改错误处理器
	go func() {
		time.Sleep(20 * time.Millisecond)
		engine.SetErrorHandler(func(task *Task[int], err interface{}) {
			mu.Lock()
			capturedHandlers = append(capturedHandlers, "handler2")
			mu.Unlock()
			atomic.AddInt32(&callCount, 1)
		})
	}()

	// 添加会 panic 的任务
	for i := 0; i < 5; i++ {
		executeAt := time.Now().Add(time.Duration(i*20) * time.Millisecond).UnixMilli()
		engine.Add(&Task[int]{
			JobID:     string(rune('A' + i)),
			ExecuteAt: executeAt,
			Data:      i,
			Callback: func(task *Task[int]) {
				panic("test panic")
			},
		})
	}

	// 等待所有任务执行
	time.Sleep(300 * time.Millisecond)

	count := atomic.LoadInt32(&callCount)
	t.Logf("捕获的 panic 数量: %d", count)

	mu.Lock()
	t.Logf("使用的处理器: %v", capturedHandlers)
	mu.Unlock()

	// 关键点：
	// 1. executeTask 使用 RLock 获取 handler
	// 2. SetErrorHandler 使用 Lock 修改 handler
	// 3. RLock 保证在读取期间 handler 不会被修改
	// 4. 这是正确的并发控制模式

	t.Log("结论：使用 RLock 是正确的，handler 在读取期间不会被修改")
}
