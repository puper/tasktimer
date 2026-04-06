package main

import (
	"fmt"
	"tasktimer"
	"time"
)

func main() {
	engine := tasktimer.New[string](400 * time.Millisecond)
	engine.Start()
	defer engine.Stop()

	// 注册主题回调函数
	engine.RegisterTopic("orders", func(task *tasktimer.Task[string]) {
		fmt.Printf("Order task executed - JobID: %s, Data: %s, ExecuteAt: %d, Topic: %s\n",
			task.JobID, task.Data, task.ExecuteAt, task.Topic)
	})

	// 添加任务时不指定回调，使用主题注册的回调
	executeAt1 := time.Now().Add(1 * time.Second).UnixMilli()
	added := engine.Add(&tasktimer.Task[string]{
		JobID:     "task1",
		Topic:     "orders",
		ExecuteAt: executeAt1,
		Data:      "order-123",
		Callback:  nil,
	})
	fmt.Printf("Add task1: %v\n", added)

	// 尝试添加重复任务
	added = engine.Add(&tasktimer.Task[string]{
		JobID:     "task1",
		Topic:     "orders",
		ExecuteAt: executeAt1,
		Data:      "order-456",
		Callback:  nil,
	})
	fmt.Printf("Add duplicate task1: %v\n", added)

	task, exists := engine.Cancel("task1")
	if exists {
		fmt.Printf("Cancelled task: %s\n", task.JobID)
	}

	// 添加多个任务
	executeAt2 := time.Now().Add(800 * time.Millisecond).UnixMilli()
	executeAt3 := time.Now().Add(1200 * time.Millisecond).UnixMilli()
	engine.Execute(func(tx *tasktimer.Tx[string]) {
		// 使用主题回调
		tx.Add(&tasktimer.Task[string]{
			JobID:     "task2",
			Topic:     "orders",
			ExecuteAt: executeAt2,
			Data:      "order-456",
			Callback:  nil,
		})
		// 使用自定义回调覆盖主题回调
		tx.Add(&tasktimer.Task[string]{
			JobID:     "task3",
			Topic:     "payments",
			ExecuteAt: executeAt3,
			Data:      "payment-789",
			Callback: func(task *tasktimer.Task[string]) {
				fmt.Printf("Custom callback - JobID: %s, Data: %s\n", task.JobID, task.Data)
			},
		})
	})

	// 使用 AddOrReplace：如果不存在则添加，存在则替换
	executeAt4 := time.Now().Add(1500 * time.Millisecond).UnixMilli()
	oldTask, replaced := engine.AddOrReplace(&tasktimer.Task[string]{
		JobID:     "task2",
		Topic:     "orders",
		ExecuteAt: executeAt4,
		Data:      "order-999",
		Callback:  nil,
	})
	fmt.Printf("AddOrReplace task2 - replaced: %v, old data: %v\n", replaced, oldTask)

	time.Sleep(2 * time.Second)

	count := engine.CancelTopic("orders")
	fmt.Printf("Cancelled %d tasks in 'orders' topic\n", count)
}
