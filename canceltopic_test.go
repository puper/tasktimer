package tasktimer

import (
	"testing"
	"time"
)

// TestCancelTopicManyTasks 测试大量任务的删除
func TestCancelTopicManyTasks(t *testing.T) {
	engine := New[int](100 * time.Millisecond)
	executeAt := time.Now().Add(500 * time.Millisecond).UnixMilli()

	// 添加 1000 个任务到同一个 topic
	engine.Execute(func(tx *Tx[int]) {
		for i := 0; i < 1000; i++ {
			jobID := string(rune(i))
			tx.Add(&Task[int]{
				JobID:     jobID,
				Topic:     "topic1",
				ExecuteAt: executeAt,
				Data:      i,
			})
		}
	})

	// 验证所有任务都添加成功
	engine.Execute(func(tx *Tx[int]) {
		if len(engine.topicIndex["topic1"]) != 1000 {
			t.Errorf("Expected 1000 tasks, got %d", len(engine.topicIndex["topic1"]))
		}
	})

	// 取消整个 topic
	count := engine.CancelTopic("topic1")

	if count != 1000 {
		t.Errorf("Expected to cancel 1000 tasks, got %d", count)
	}

	// 验证所有任务都被删除
	engine.Execute(func(tx *Tx[int]) {
		if _, exists := engine.topicIndex["topic1"]; exists {
			t.Error("topic1 should be deleted from topicIndex")
		}
		if len(engine.jobToSlot) != 0 {
			t.Errorf("jobToSlot should be empty, got %d tasks", len(engine.jobToSlot))
		}
	})
}

// TestCancelTopicEmpty 测试空 topic
func TestCancelTopicEmpty(t *testing.T) {
	engine := New[int](100 * time.Millisecond)

	// 取消不存在的 topic
	count := engine.CancelTopic("nonexistent")
	if count != 0 {
		t.Errorf("Expected 0 for nonexistent topic, got %d", count)
	}

	// 创建一个空 topic（手动操作）
	engine.Execute(func(tx *Tx[int]) {
		engine.topicIndex["empty"] = make(map[string]struct{})
	})

	count = engine.CancelTopic("empty")
	if count != 0 {
		t.Errorf("Expected 0 for empty topic, got %d", count)
	}
}

// TestCancelTopicConsistency 测试数据一致性
func TestCancelTopicConsistency(t *testing.T) {
	engine := New[int](100 * time.Millisecond)
	executeAt := time.Now().Add(500 * time.Millisecond).UnixMilli()

	// 添加多个 topic 的任务
	engine.Execute(func(tx *Tx[int]) {
		// topic1: 3 个任务
		for i := 1; i <= 3; i++ {
			tx.Add(&Task[int]{
				JobID:     string(rune('a' + i)),
				Topic:     "topic1",
				ExecuteAt: executeAt,
				Data:      i,
			})
		}
		// topic2: 2 个任务
		for i := 1; i <= 2; i++ {
			tx.Add(&Task[int]{
				JobID:     string(rune('x' + i)),
				Topic:     "topic2",
				ExecuteAt: executeAt,
				Data:      i * 10,
			})
		}
	})

	// 取消 topic1
	count := engine.CancelTopic("topic1")
	if count != 3 {
		t.Errorf("Expected to cancel 3 tasks, got %d", count)
	}

	// 验证数据结构的一致性
	engine.Execute(func(tx *Tx[int]) {
		// topic1 应该完全消失
		if _, exists := engine.topicIndex["topic1"]; exists {
			t.Error("topic1 should be deleted")
		}

		// topic2 应该保持不变
		if len(engine.topicIndex["topic2"]) != 2 {
			t.Errorf("topic2 should have 2 tasks, got %d", len(engine.topicIndex["topic2"]))
		}

		// jobToSlot 应该只有 topic2 的任务
		if len(engine.jobToSlot) != 2 {
			t.Errorf("jobToSlot should have 2 tasks, got %d", len(engine.jobToSlot))
		}
	})
}
