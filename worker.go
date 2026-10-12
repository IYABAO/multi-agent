package main

import (
	"context"
	"fmt"
	"strings"
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

// RunWithContext 执行审查类任务，把其他子任务的实际输出作为上下文注入
func (w *WorkerAgent) RunWithContext(ctx context.Context, task Task, tasks []Task, otherResults []Result) (string, error) {
	taskMap := map[string]Task{}
	for _, t := range tasks {
		taskMap[t.ID] = t
	}

	var sb strings.Builder
	for _, res := range otherResults {
		if res.TaskID == task.ID || res.Content == "" || strings.HasPrefix(res.Content, "ERROR:") {
			continue
		}
		if t, ok := taskMap[res.TaskID]; ok {
			sb.WriteString(fmt.Sprintf("【子任务%s：%s 的实际输出】\n%s\n\n", res.TaskID, t.Title, res.Content))
		}
	}

	prompt := fmt.Sprintf("子任务：%s\n要求：%s\n\n以下是其他子任务已经产出的实际内容，请基于这些真实文本进行审查、评估或分析，不要泛泛而谈：\n\n%s\n请完成这个子任务。",
		task.Title, task.Description, sb.String())
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
		"general": NewWorker(cfg, "General-Worker", "通用执行"),
		"writer":  NewWorker(cfg, "Writer", "技术写作与内容组织"),
		"coder":   NewWorker(cfg, "Coder", "代码实现与技术方案"),
		"critic":  NewWorker(cfg, "Critic", "审查与批判性分析"),
	}
}

// isReviewTask 判断是否为审查/评估类任务（需要等其他任务完成后才能执行）
func isReviewTask(t Task) bool {
	assigned := strings.ToLower(t.AssignedTo)
	if assigned == "critic" || assigned == "reviewer" || assigned == "review" {
		return true
	}
	text := strings.ToLower(t.Title + " " + t.Description)
	for _, kw := range []string{"审查", "评估", "审核", "校对", "review", "critique", "critique"} {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
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

// RunWorkers 两阶段执行：先并行跑普通任务，再串行跑审查任务（注入其他任务输出）
func RunWorkers(ctx context.Context, cfg *Config, tasks []Task, pool map[string]*WorkerAgent, maxRetries int) []Result {
	results := make([]Result, len(tasks))
	var normalIdx, reviewIdx []int

	for i, t := range tasks {
		if isReviewTask(t) {
			reviewIdx = append(reviewIdx, i)
		} else {
			normalIdx = append(normalIdx, i)
		}
	}

	// 第一阶段：并行执行普通任务
	var wg sync.WaitGroup
	for _, i := range normalIdx {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			t := tasks[idx]
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
		}(i)
	}
	wg.Wait()

	// 第二阶段：审查任务，注入其他任务的实际输出
	for _, i := range reviewIdx {
		t := tasks[i]
		worker := selectWorker(pool, t)
		content, err := worker.RunWithContext(ctx, t, tasks, results)
		if err != nil {
			content = fmt.Sprintf("ERROR: %v", err)
		}
		results[i] = Result{
			TaskID:    t.ID,
			Agent:     worker.Name(),
			Content:   content,
			CreatedAt: time.Now(),
		}
	}

	return results
}

// RetryFailed 对未通过的任务重试；审查类任务重试时注入其他任务输出
func RetryFailed(ctx context.Context, cfg *Config, tasks []Task, results []Result, pool map[string]*WorkerAgent, maxRetries int) []Result {
	taskMap := map[string]Task{}
	for _, t := range tasks {
		taskMap[t.ID] = t
	}

	for i, res := range results {
		if res.Approved || res.Score >= 7 {
			continue
		}
		task := taskMap[res.TaskID]
		worker := selectWorker(pool, task)
		for attempt := 1; attempt <= maxRetries; attempt++ {
			fmt.Printf("🔄 [%s] 重试第 %d 次 (上次得分 %d)\n", task.ID, attempt, res.Score)

			var newContent string
			var err error
			if isReviewTask(task) {
				// 审查任务重试：注入其他任务的实际输出
				newContent, err = worker.RunWithContext(ctx, task, tasks, results)
			} else {
				retryPrompt := fmt.Sprintf("子任务：%s\n要求：%s\n上次结果被审查打分为 %d，反馈：%s\n请改进后重新完成。",
					task.Title, task.Description, res.Score, res.Feedback)
				newContent, err = callLLM(cfg, worker.SystemPrompt(), retryPrompt)
			}

			if err == nil && newContent != "" {
				results[i].Content = newContent
				results[i].Feedback = fmt.Sprintf("重试%d后更新", attempt)
				break
			}
		}
	}
	return results
}
