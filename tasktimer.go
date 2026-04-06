package tasktimer

import (
	"sync"
	"time"
)

// Task 表示一个延迟任务的定义
// T 是任务数据的泛型类型，可以存储任意类型的数据
type Task[T any] struct {
	JobID     string         // 任务唯一标识符
	Topic     string         // 任务主题，用于批量管理相关任务
	ExecuteAt int64          // 任务执行时间戳（毫秒）
	Data      T              // 任务携带的数据
	Callback  func(*Task[T]) // 任务回调函数，参数为完整的 Task 对象
}

// Engine 是延迟任务调度引擎的核心结构
// T 是任务数据的泛型类型
// Engine 使用时间槽（time slot）机制实现高效的延迟任务调度
type Engine[T any] struct {
	mu sync.RWMutex // 读写锁，保证并发安全

	slots          map[int64]map[string]*Task[T]  // 时间槽数据，key 为时间槽的开始时间戳，value 为该时间槽内的任务映射
	jobToSlot      map[string]int64               // 任务 ID 到时间槽的映射，用于快速查找任务所在的时间槽
	topicIndex     map[string]map[string]struct{} // 主题索引，key 为主题名称，value 为该主题下的任务 ID 集合
	topicCallbacks map[string]func(*Task[T])      // 主题回调函数映射，key 为主题名称，value 为该主题的默认回调函数

	resolution        int64                       // 时间精度（毫秒），决定时间槽的大小
	running           bool                        // 引擎运行状态标志
	stopChan          chan struct{}               // 停止信号通道
	errorHandler      func(*Task[T], interface{}) // 错误回调函数，用于处理任务执行中的 panic
	workerPool        chan struct{}               // 工作池，限制并发 goroutine 数量
	lastProcessedSlot int64                       // 上次处理的时间槽 key，用于优化扫描范围
}

// Tx 是事务对象，用于执行原子性的任务操作
// T 是任务数据的泛型类型
type Tx[T any] struct {
	engine *Engine[T] // 关联的引擎实例
}

// New 创建一个新的延迟任务调度引擎
// res 参数指定时间精度，即时间槽的大小
// 如果 res <= 0，则使用默认值 400 毫秒
// maxWorkers 参数指定最大并发 goroutine 数量，用于限制资源使用
// 如果 maxWorkers <= 0，则使用默认值 100
// 返回初始化后的引擎实例
func New[T any](res time.Duration, maxWorkers ...int) *Engine[T] {
	if res <= 0 {
		res = 400 * time.Millisecond
	}

	workers := 100
	if len(maxWorkers) > 0 && maxWorkers[0] > 0 {
		workers = maxWorkers[0]
	}

	return &Engine[T]{
		slots:          make(map[int64]map[string]*Task[T]),
		jobToSlot:      make(map[string]int64),
		topicIndex:     make(map[string]map[string]struct{}),
		topicCallbacks: make(map[string]func(*Task[T])),
		resolution:     int64(res / time.Millisecond),
		running:        false,
		stopChan:       make(chan struct{}),
		workerPool:     make(chan struct{}, workers),
	}
}

// Execute 在事务中执行任务操作
// fn 参数是一个回调函数，接收 Tx 对象作为参数
// 此方法会获取互斥锁，确保操作的原子性和线程安全
func (e *Engine[T]) Execute(fn func(tx *Tx[T])) {
	e.mu.Lock()
	defer e.mu.Unlock()
	tx := &Tx[T]{engine: e}
	fn(tx)
}

// RegisterTopic 注册主题及其默认回调函数
// topic: 主题名称
// callback: 该主题下任务的默认回调函数
// 如果主题已注册，会覆盖之前的回调函数
func (e *Engine[T]) RegisterTopic(topic string, callback func(*Task[T])) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.topicCallbacks[topic] = callback
}

// SetErrorHandler 设置错误回调函数
// 当任务执行过程中发生 panic 时，会调用此回调函数
// handler 参数接收任务对象和 panic 值
func (e *Engine[T]) SetErrorHandler(handler func(*Task[T], interface{})) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errorHandler = handler
}

// Add 向事务中添加一个新的延迟任务（Tx 方法）
// task: 任务对象，必须设置 JobID 和 ExecuteAt
// 如果 task.Callback 为 nil，则使用主题注册的回调函数
// 如果任务已存在，不会添加新任务，返回 false
// 如果任务的时间槽已过期，会立即执行任务并返回 true
// 返回是否成功添加（true 表示添加成功或已立即执行，false 表示任务已存在）
func (tx *Tx[T]) Add(task *Task[T]) bool {
	e := tx.engine

	// 检查任务是否已存在
	if _, exists := e.jobToSlot[task.JobID]; exists {
		return false
	}

	// 根据时间精度计算所属的时间槽
	slotKey := (task.ExecuteAt / e.resolution) * e.resolution

	// 如果未指定回调函数，尝试使用主题注册的回调函数
	if task.Callback == nil && task.Topic != "" {
		task.Callback = e.topicCallbacks[task.Topic]
	}

	// 检查时间槽是否已过期或已被处理（修复任务丢失 Bug）
	now := time.Now().UnixMilli()
	currentSlotKey := (now / e.resolution) * e.resolution

	// 检查三种情况：
	// 1. 时间槽已过期（slotKey < currentSlotKey）
	// 2. 任务执行时间已过（task.ExecuteAt <= now）
	// 3. 时间槽已被 Tick 处理过（slotKey <= lastProcessedSlot）
	if slotKey < currentSlotKey || task.ExecuteAt <= now || slotKey <= e.lastProcessedSlot {
		// 任务已过期或应该立即执行
		go e.executeTask(task)
		return true
	}

	// 将任务添加到对应的时间槽
	if e.slots[slotKey] == nil {
		e.slots[slotKey] = make(map[string]*Task[T])
	}
	e.slots[slotKey][task.JobID] = task
	e.jobToSlot[task.JobID] = slotKey

	// 如果指定了主题，更新主题索引
	if task.Topic != "" {
		if e.topicIndex[task.Topic] == nil {
			e.topicIndex[task.Topic] = make(map[string]struct{})
		}
		e.topicIndex[task.Topic][task.JobID] = struct{}{}
	}

	return true
}

// Get 从事务中获取指定任务（Tx 方法）
// jobID: 任务唯一标识符
// 返回任务对象和是否存在的布尔值
func (tx *Tx[T]) Get(jobID string) (*Task[T], bool) {
	e := tx.engine
	// 查找任务所在的时间槽
	slotKey, exists := e.jobToSlot[jobID]
	if !exists {
		return nil, false
	}
	// 获取时间槽
	slot, exists := e.slots[slotKey]
	if !exists {
		return nil, false
	}
	// 获取任务
	task, exists := slot[jobID]
	return task, exists
}

// Delete 从事务中删除指定任务（Tx 方法）
// jobID: 任务唯一标识符
// 返回被删除的任务对象和是否成功的布尔值
func (tx *Tx[T]) Delete(jobID string) (*Task[T], bool) {
	e := tx.engine
	// 查找任务所在的时间槽
	slotKey, exists := e.jobToSlot[jobID]
	if !exists {
		return nil, false
	}

	// 获取时间槽
	slot, exists := e.slots[slotKey]
	if !exists {
		return nil, false
	}

	// 获取任务
	task, exists := slot[jobID]
	if !exists {
		return nil, false
	}

	// 从时间槽中删除任务
	delete(slot, jobID)
	// 如果时间槽为空，删除该时间槽
	if len(slot) == 0 {
		delete(e.slots, slotKey)
	}

	// 删除任务 ID 到时间槽的映射
	delete(e.jobToSlot, jobID)

	// 如果任务有主题，更新主题索引
	if task.Topic != "" {
		if topicJobs, exists := e.topicIndex[task.Topic]; exists {
			delete(topicJobs, jobID)
			// 如果主题下没有任务，删除该主题索引
			if len(topicJobs) == 0 {
				delete(e.topicIndex, task.Topic)
			}
		}
	}

	return task, true
}

// Add 向引擎添加一个新的延迟任务（Engine 方法）
// 这是 Add 的便捷封装方法，内部自动创建事务
// task: 任务对象，必须设置 JobID 和 ExecuteAt
// 如果 task.Callback 为 nil，则使用主题注册的回调函数
// 如果任务已存在，不会添加新任务，返回 false
// 返回是否成功添加（true 表示添加成功，false 表示任务已存在）
func (e *Engine[T]) Add(task *Task[T]) bool {
	var added bool
	e.Execute(func(tx *Tx[T]) {
		added = tx.Add(task)
	})
	return added
}

// Get 从引擎获取指定任务（Engine 方法）
// 这是 Get 的便捷封装方法，内部自动创建事务
// jobID: 任务唯一标识符
// 返回任务对象和是否存在的布尔值
func (e *Engine[T]) Get(jobID string) (*Task[T], bool) {
	var task *Task[T]
	var exists bool
	e.Execute(func(tx *Tx[T]) {
		task, exists = tx.Get(jobID)
	})
	return task, exists
}

// Delete 从引擎删除指定任务（Engine 方法）
// 这是 Delete 的便捷封装方法，内部自动创建事务
// jobID: 任务唯一标识符
// 返回被删除的任务对象和是否成功的布尔值
func (e *Engine[T]) Delete(jobID string) (*Task[T], bool) {
	var task *Task[T]
	var exists bool
	e.Execute(func(tx *Tx[T]) {
		task, exists = tx.Delete(jobID)
	})
	return task, exists
}

// Replace 替换指定任务
// task: 新的任务对象，JobID 必须与要替换的任务一致
// 返回旧任务对象和是否存在的布尔值
// 如果任务不存在，不会创建新任务
func (e *Engine[T]) Replace(task *Task[T]) (*Task[T], bool) {
	var oldTask *Task[T]
	var exists bool

	e.Execute(func(tx *Tx[T]) {
		oldTask, exists = tx.Delete(task.JobID)
		if exists {
			tx.Add(task)
		}
	})

	return oldTask, exists
}

// AddOrReplace 添加或替换任务
// 如果任务不存在则添加，如果存在则替换
// task: 任务对象，必须设置 JobID 和 ExecuteAt
// 返回旧任务对象（如果存在）和是否替换的布尔值
func (e *Engine[T]) AddOrReplace(task *Task[T]) (*Task[T], bool) {
	var oldTask *Task[T]
	var replaced bool

	e.Execute(func(tx *Tx[T]) {
		oldTask, replaced = tx.Delete(task.JobID)
		tx.Add(task)
	})

	return oldTask, replaced
}

// Cancel 取消指定任务
// jobID: 任务唯一标识符
// 返回被取消的任务对象和是否成功的布尔值
// 内部自动创建事务执行删除操作
func (e *Engine[T]) Cancel(jobID string) (*Task[T], bool) {
	var task *Task[T]
	var exists bool
	e.Execute(func(tx *Tx[T]) {
		task, exists = tx.Delete(jobID)
	})
	return task, exists
}

// CancelTopic 取消指定主题下的所有任务
// topic: 任务主题名称
// 返回被取消的任务数量
// 如果主题不存在或主题下没有任务，返回 0
func (e *Engine[T]) CancelTopic(topic string) int {
	var count int
	e.Execute(func(tx *Tx[T]) {
		jobIDs, exists := e.topicIndex[topic]
		if !exists {
			return
		}

		for jobID := range jobIDs {
			if _, ok := tx.Delete(jobID); ok {
				count++
			}
		}
	})
	return count
}

// executeTask 执行单个任务，处理 panic 恢复和并发限制
// 这是一个内部辅助方法
func (e *Engine[T]) executeTask(task *Task[T]) {
	// 获取工作池槽位（限制并发）
	e.workerPool <- struct{}{}
	defer func() { <-e.workerPool }()

	// 恢复 panic
	defer func() {
		if r := recover(); r != nil {
			// 调用错误处理器（如果已设置）
			e.mu.RLock()
			handler := e.errorHandler
			e.mu.RUnlock()

			if handler != nil {
				handler(task, r)
			}
		}
	}()

	// 执行任务回调
	if task.Callback != nil {
		task.Callback(task)
	}
}

// Start 启动任务调度引擎
// 启动后，引擎会按照设定的精度定期检查并执行到期任务
// 如果引擎已经在运行，则不会重复启动
func (e *Engine[T]) Start() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.stopChan = make(chan struct{})
	e.mu.Unlock()

	go e.run()
}

// Stop 停止任务调度引擎
// 停止后，引擎不再检查和执行任务
// 如果引擎已经停止，则不会执行任何操作
func (e *Engine[T]) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.running = false
	close(e.stopChan)
	e.mu.Unlock()
}

// run 是引擎的主运行循环
// 定期触发 Tick 方法检查并执行到期任务
// 当收到停止信号时退出
func (e *Engine[T]) run() {
	ticker := time.NewTicker(time.Duration(e.resolution) * time.Millisecond)
	defer ticker.Stop()

	e.mu.RLock()
	stopChan := e.stopChan
	e.mu.RUnlock()

	for {
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			e.Tick()
		}
	}
}

// Tick 检查并执行所有已到期时间槽内的任务
// 此方法由引擎定时调用，也可手动调用进行测试
// 执行任务时会进行以下操作：
// 1. 计算从上次 tick 到当前 tick 之间的时间槽范围
// 2. 获取该范围内所有时间槽的任务
// 3. 从数据结构中移除这些任务
// 4. 异步执行任务回调函数（受工作池限制）
// 5. 捕获并恢复回调函数中的 panic，通过错误处理器通知用户
func (e *Engine[T]) Tick() {
	now := time.Now().UnixMilli()
	currentSlotKey := (now / e.resolution) * e.resolution

	e.mu.Lock()
	defer e.mu.Unlock()

	// 确定扫描范围的起始点
	var startSlotKey int64
	if e.lastProcessedSlot == 0 {
		// 首次 tick：从当前时间槽开始
		startSlotKey = currentSlotKey
	} else {
		// 后续 tick：从上次处理的下一个槽开始
		// 这样可以处理 tick 延迟导致的错过的时间槽
		startSlotKey = e.lastProcessedSlot + e.resolution
	}

	// 收集时间范围内的所有任务
	var tasksToExecute []*Task[T]

	// 扫描 [startSlotKey, currentSlotKey] 范围内的槽
	// 注意：如果 startSlotKey > currentSlotKey（例如时间调整），不执行任何操作
	if startSlotKey <= currentSlotKey {
		for slotKey := startSlotKey; slotKey <= currentSlotKey; slotKey += e.resolution {
			slot, exists := e.slots[slotKey]
			if !exists {
				continue
			}

			// 收集该时间槽内的所有任务
			for _, task := range slot {
				tasksToExecute = append(tasksToExecute, task)
			}

			// 删除该时间槽
			delete(e.slots, slotKey)
		}

		// 更新最后处理的时间槽
		e.lastProcessedSlot = currentSlotKey
	}

	// 清理已执行任务的索引
	for _, task := range tasksToExecute {
		delete(e.jobToSlot, task.JobID)
		if task.Topic != "" {
			if topicJobs, exists := e.topicIndex[task.Topic]; exists {
				delete(topicJobs, task.JobID)
				if len(topicJobs) == 0 {
					delete(e.topicIndex, task.Topic)
				}
			}
		}
	}

	// 异步执行任务回调（受工作池限制）
	for _, task := range tasksToExecute {
		go e.executeTask(task)
	}
}
