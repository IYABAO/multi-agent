package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// WorkerAgent 执行单个子任务
type WorkerAgent struct {
	cfg  *Config
	name string
	role string
}

func NewWorker(cfg *Config, name, role string) *WorkerAgent {
	return &WorkerAgent{cfg: cfg, name: name, role: role}
}

func (w *WorkerAgent) Name() string { return w.name }

func (w *WorkerAgent) SystemPrompt() string {
	return fmt.Sprintf("你是一个%s专家。专注完成分配给你的子任务，给出具体、可执行、高质量的结果。", w.role)
}

func (w *WorkerAgent) Run(ctx context.Context, task Task) (string, error) {
	prompt := fmt.Sprintf("子任务：%s\n要求：%s\n\n请完成这个子任务。", task.Title, task.Description)
	return callLLM(w.cfg, w.SystemPrompt(), prompt)
}

func (w *WorkerAgent) RunStream(ctx context.Context, task Task) (string, error) {
	prompt := fmt.Sprintf("子任务：%s\n要求：%s\n\n请完成这个子任务。", task.Title, task.Description)
	fmt.Printf("\n--- %s 流式输出 ---\n", w.Name())
	return callLLMStream(w.cfg, w.SystemPrompt(), prompt, w.Name())
}

// 默认专家角色
func DefaultWorkerPool(cfg *Config) map[string]*WorkerAgent {
	return map[string]*WorkerAgent{
		"general":   NewWorker(cfg, "General-Worker", "通用执行"),
		"writer":    NewWorker(cfg, "Writer", "技术写作与内容组织"),
		"coder":     NewWorker(cfg, "Coder", "代码实现与技术方案"),
		"reviewer2": NewWorker(cfg, "Critic", "审查与批判性分析"),
	}
}

// selectWorker 根据任务的 AssignedTo 字段选 worker
func selectWorker(pool map[string]*WorkerAgent, task Task) *WorkerAgent {
	if task.AssignedTo != "" {
		if w, ok := pool[task.AssignedTo]; ok {
			return w
		}
	}
	return pool["general"]
}

// RunWorkers 并行执行所有子任务，支持重试
func RunWorkers(ctx context.Context, cfg *Config, tasks []Task, pool map[string]*WorkerAgent, maxRetries int) []Result {
	results := make([]Result, len(tasks))
	var wg sync.WaitGroup

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t Task) {
			defer wg.Done()
			worker := selectWorker(pool, t)

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

// RetryFailed 对未通过的任务重试
func RetryFailed(ctx context.Context, cfg *Config, tasks []Task, results []Result, pool map[string]*WorkerAgent, maxRetries int) []Result {
	taskMap := map[string]Task{}
	for _, t := range tasks {
		taskMap[t.ID] = t
	}

	for i, res := range results {
		if res.Approved || res.Score >= 7 {
			continue
		}
		// 未通过，重试
		task := taskMap[res.TaskID]
		for attempt := 1; attempt <= maxRetries; attempt++ {
			fmt.Printf("🔄 [%s] 重试第 %d 次 (上次得分 %d)\n", task.ID, attempt, res.Score)
			worker := selectWorker(pool, task)
			retryPrompt := fmt.Sprintf("子任务：%s\n要求：%s\n上次结果被审查打分为 %d，反馈：%s\n请改进后重新完成。",
				task.Title, task.Description, res.Score, res.Feedback)
			newContent, err := callLLM(cfg, worker.SystemPrompt(), retryPrompt)
			if err == nil && newContent != "" {
				results[i].Content = newContent
				results[i].Feedback = fmt.Sprintf("重试%d后更新", attempt)
				break
			}
		}
	}
	return results
}
