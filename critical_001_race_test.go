package tasktimer

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestCRITICAL001_StopStartRace 复现 CRITICAL-001 数据竞争
// 此测试演示 Stop/Start 重启时的竞态条件
func TestCRITICAL001_StopStartRace(t *testing.T) {
	engine := New[string](100 * time.Millisecond)

	var panicCount int32

	// 并发执行 Stop/Start 循环
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				engine.Start()
				time.Sleep(time.Microsecond * 100)

				defer func() {
					if r := recover(); r != nil {
						atomic.AddInt32(&panicCount, 1)
						t.Logf("Panic detected: %v", r)
					}
				}()

				engine.Stop()
				time.Sleep(time.Microsecond * 100)
			}
		}()
	}

	time.Sleep(2 * time.Second)

	if count := atomic.LoadInt32(&panicCount); count > 0 {
		t.Errorf("CRITICAL-001 复现: 发生 %d 次 panic", count)
	}
}

// TestCRITICAL001_RaceDetector 使用 race detector 验证竞态条件
// 运行方式: go test -race -run TestCRITICAL001_RaceDetector
func TestCRITICAL001_RaceDetector(t *testing.T) {
	engine := New[int](50 * time.Millisecond)

	// 启动引擎
	engine.Start()
	time.Sleep(100 * time.Millisecond)

	// 停止引擎
	engine.Stop()
	time.Sleep(100 * time.Millisecond)

	// 重新启动引擎（此时可能存在竞态）
	engine.Start()
	time.Sleep(100 * time.Millisecond)

	// 再次停止（此时可能 panic）
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("CRITICAL-001: panic during Stop/Start cycle: %v", r)
		}
	}()

	engine.Stop()
}

// TestCRITICAL001_ConcurrentStopStart 测试并发 Stop/Start 的安全性
func TestCRITICAL001_ConcurrentStopStart(t *testing.T) {
	engine := New[string](100 * time.Millisecond)

	var startCount int32
	var stopCount int32

	// 启动多个 goroutine 并发执行 Start/Stop
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 50; j++ {
				engine.Start()
				atomic.AddInt32(&startCount, 1)
				time.Sleep(time.Millisecond * time.Duration(id%5+1))

				engine.Stop()
				atomic.AddInt32(&stopCount, 1)
				time.Sleep(time.Millisecond * time.Duration(id%5+1))
			}
		}(i)
	}

	time.Sleep(3 * time.Second)

	t.Logf("Start 调用次数: %d, Stop 调用次数: %d",
		atomic.LoadInt32(&startCount),
		atomic.LoadInt32(&stopCount))
}
