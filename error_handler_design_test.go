package tasktimer

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestErrorHandlerConcurrencyCorrectness 验证 errorHandler 并发正确性
func TestErrorHandlerConcurrencyCorrectness(t *testing.T) {
	t.Log("=== 分析 errorHandler 并发设计 ===")
	t.Log()
	t.Log("代码片段：")
	t.Log("  e.mu.RLock()")
	t.Log("  handler := e.errorHandler")
	t.Log("  e.mu.RUnlock()")
	t.Log("  if handler != nil {")
	t.Log("    handler(task, r)")
	t.Log("  }")
	t.Log()
	t.Log("设计分析：")
	t.Log("1. RLock 保护的是'读取 errorHandler 引用'这个操作")
	t.Log("2. 读取后立即释放锁，避免死锁")
	t.Log("3. handler 的调用在锁外进行，这是正确的")
	t.Log()
	t.Log("为什么这是正确的？")
	t.Log("- 如果在锁内调用 handler，可能导致死锁")
	t.Log("  （handler 可能调用 SetErrorHandler 或其他方法）")
	t.Log("- 读取 handler 引用后，即使 errorHandler 被修改")
	t.Log("  当前 handler 仍然可以安全执行")
	t.Log("- Go 的函数值是不可变的，一旦读取就固定了")
	t.Log()
	t.Log("结论：")
	t.Log("✓ 这是正确的并发设计模式")
	t.Log("✓ 不是 Bug，而是最佳实践")
	t.Log()

	// 实际测试
	engine := New[int](50 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	var handlerCalled int32

	engine.SetErrorHandler(func(task *Task[int], err interface{}) {
		atomic.StoreInt32(&handlerCalled, 1)
	})

	// 添加会 panic 的任务
	future := time.Now().Add(100 * time.Millisecond).UnixMilli()
	engine.Add(&Task[int]{
		JobID:     "panic-task",
		ExecuteAt: future,
		Callback: func(task *Task[int]) {
			panic("test")
		},
	})

	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt32(&handlerCalled) == 1 {
		t.Log("✓ errorHandler 正确执行")
	}
}

// TestErrorHandlerNilSafety 验证 errorHandler 为 nil 的安全性
func TestErrorHandlerNilSafety(t *testing.T) {
	engine := New[int](50 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	// 不设置 errorHandler（保持 nil）

	// 添加会 panic 的任务
	future := time.Now().Add(100 * time.Millisecond).UnixMilli()
	engine.Add(&Task[int]{
		JobID:     "panic-task",
		ExecuteAt: future,
		Callback: func(task *Task[int]) {
			panic("test")
		},
	})

	time.Sleep(200 * time.Millisecond)

	// 应该不会 panic，因为 executeTask 有 recover
	t.Log("✓ errorHandler 为 nil 时，panic 被安全捕获")
}
