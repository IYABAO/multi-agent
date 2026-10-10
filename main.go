package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func stringRepeat(s string, n int) string {
	return strings.Repeat(s, n)
}

func main() {
	task := flag.String("task", "", "要执行的复杂任务")
	baseURL := flag.String("base", "https://api.deepseek.com/v1", "LLM API base URL")
	model := flag.String("model", "deepseek-chat", "模型名")
	stream := flag.Bool("stream", false, "流式输出")
	maxRetries := flag.Int("retries", 2, "未通过任务最大重试次数")
	flag.Parse()

	if *task == "" {
		fmt.Fprintln(os.Stderr, "用法: multi-agent --task \"你的复杂任务\" [--stream] [--retries 2]")
		os.Exit(1)
	}

	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "请设置 DEEPSEEK_API_KEY 环境变量")
		os.Exit(1)
	}

	cfg := &Config{BaseURL: *baseURL, APIKey: apiKey, Model: *model}
	ctx := context.Background()
	sep := stringRepeat("=", 60)

	// 1. Planner 拆解任务
	fmt.Println("🔍 [Planner] 正在拆解任务...")
	planner := &PlannerAgent{cfg: cfg}
	tasks, err := planner.Run(ctx, *task)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Planner error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("📋 拆解为 %d 个子任务：\n", len(tasks))
	for _, t := range tasks {
		fmt.Printf("   [%s] %s → 角色: %s\n", t.ID, t.Title, t.AssignedTo)
	}

	// 2. Worker 并行执行
	fmt.Println("\n⚡ [Worker] 并行执行中...")
	pool := DefaultWorkerPool(cfg)
	var results []Result
	if *stream {
		// 流式模式：逐个 worker 流式输出（并行 goroutine 中流式会交错，简化为顺序）
		for _, t := range tasks {
			worker := selectWorker(pool, t)
			content, err := worker.RunStream(ctx, t)
			if err != nil {
				content = fmt.Sprintf("ERROR: %v", err)
			}
			results = append(results, Result{
				TaskID: t.ID, Agent: worker.Name(), Content: content, CreatedAt: time.Now(),
			})
		}
	} else {
		results = RunWorkers(ctx, cfg, tasks, pool, *maxRetries)
		for _, res := range results {
			fmt.Printf("   ✅ [%s] %s (完成)\n", res.TaskID, res.Agent)
		}
	}

	// 3. Reviewer 审查
	fmt.Println("\n🔍 [Reviewer] 审查结果...")
	reviewer := &ReviewerAgent{cfg: cfg}
	results, err = reviewer.Run(ctx, results, tasks)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Reviewer warning: %v\n", err)
	}

	// 4. 重试未通过的任务
	failedCount := 0
	for _, res := range results {
		if !res.Approved && res.Score < 7 {
			failedCount++
		}
	}
	if failedCount > 0 && *maxRetries > 0 {
		fmt.Printf("\n🔄 有 %d 个任务未通过，开始重试...\n", failedCount)
		results = RetryFailed(ctx, cfg, tasks, results, pool, *maxRetries)
		// 重新审查
		results, _ = reviewer.Run(ctx, results, tasks)
	}

	// 打印审查结果
	approved, failed := 0, 0
	for _, res := range results {
		status := "✅"
		if !res.Approved {
			status = "❌"
			failed++
		} else {
			approved++
		}
		fmt.Printf("   %s [%s] score=%d feedback=%s\n", status, res.TaskID, res.Score, res.Feedback)
	}

	// 5. 汇总
	fmt.Println("\n📝 [Reviewer] 生成最终交付物...")
	final, err := reviewer.Summarize(ctx, *task, results)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Summarize error: %v\n", err)
		final = "汇总失败"
	}

	fmt.Println("\n" + sep)
	fmt.Println("最终交付物：")
	fmt.Println(sep)
	fmt.Println(final)
	fmt.Println("\n" + sep)
	fmt.Printf("统计：%d 通过 / %d 未通过 / %d 总任务\n", approved, failed, len(tasks))
}
