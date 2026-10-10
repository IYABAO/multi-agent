package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// WorkerAgent 执行单个子任务
type WorkerAgent struct {
	cfg   *Config
	name  string
	role  string
}

func NewWorker(cfg *Config, name, role string) *WorkerAgent {
	return &WorkerAgent{cfg: cfg, name: name, role: role}
}

func (w *WorkerAgent) Name() string { return w.name }

func (w *WorkerAgent) SystemPrompt() string {
	return fmt.Sprintf("你是一个%s专家。专注完成分配给你的子任务，给出具体、可执行的结果。", w.role)
}

func (w *WorkerAgent) Run(ctx context.Context, task Task) (string, error) {
	prompt := fmt.Sprintf("子任务：%s\n要求：%s\n\n请完成这个子任务。", task.Title, task.Description)
	return callLLM(w.cfg, w.SystemPrompt(), prompt)
}

// RunWorkers 并行执行所有子任务
func RunWorkers(ctx context.Context, cfg *Config, tasks []Task, workerPool map[string]*WorkerAgent) []Result {
	results := make([]Result, len(tasks))
	var wg sync.WaitGroup

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t Task) {
			defer wg.Done()

			// 轮询选择 worker
			worker := workerPool["general"]
			if w, ok := workerPool[t.AssignedTo]; ok {
				worker = w
			}

			content, err := worker.Run(ctx, t)
			if err != nil {
				content = fmt.Sprintf("ERROR: %v", err)
			}
			results[idx] = Result{
				TaskID:    t.ID,
				Agent:     worker.Name(),
				Content:   content,
				CreatedAt: time.Now(),
			}
		}(i, task)
	}

	wg.Wait()
	return results
}
