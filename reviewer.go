package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ReviewerAgent 审查 Worker 的结果
type ReviewerAgent struct {
	cfg *Config
}

func (r *ReviewerAgent) Name() string { return "Reviewer" }

func (r *ReviewerAgent) SystemPrompt() string {
	return `你是质量审查专家。评估每个子任务的完成质量。
对每个结果输出一行 JSON（不要输出数组，每行一个对象）：
{"task_id":"1","score":8,"approved":true,"feedback":"简述意见"}
score 范围 1-10，>=7 为通过。不要输出其他内容。`
}

// parseReviewNDJSON 从 LLM 返回的 NDJSON 文本中解析评分结果
func parseReviewNDJSON(resp string) map[string]reviewScore {
	type review struct {
		TaskID   string `json:"task_id"`
		Score    int    `json:"score"`
		Approved bool   `json:"approved"`
		Feedback string `json:"feedback"`
	}
	reviewMap := map[string]reviewScore{}
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var rv review
		if err := json.Unmarshal([]byte(line), &rv); err != nil {
			continue
		}
		if rv.TaskID != "" {
			reviewMap[rv.TaskID] = reviewScore{
				Score:    rv.Score,
				Approved: rv.Approved,
				Feedback: rv.Feedback,
			}
		}
	}
	return reviewMap
}

type reviewScore struct {
	Score    int
	Approved bool
	Feedback string
}

func (r *ReviewerAgent) Run(ctx context.Context, results []Result, tasks []Task) ([]Result, error) {
	taskMap := map[string]Task{}
	for _, t := range tasks {
		taskMap[t.ID] = t
	}

	prompt := "以下是各子任务的执行结果，请审查：\n\n"
	for _, res := range results {
		task := taskMap[res.TaskID]
		prompt += fmt.Sprintf("【子任务%s】%s\n结果：%s\n\n", res.TaskID, task.Title, res.Content)
	}

	resp, err := callLLM(r.cfg, r.SystemPrompt(), prompt)
	if err != nil {
		return results, err
	}

	reviewMap := parseReviewNDJSON(resp)
	if len(reviewMap) == 0 {
		return results, fmt.Errorf("no valid review parsed, raw: %s", resp)
	}

	for i, res := range results {
		if rv, ok := reviewMap[res.TaskID]; ok {
			results[i].Score = rv.Score
			results[i].Approved = rv.Approved
			results[i].Feedback = rv.Feedback
		}
	}
	return results, nil
}

// Summarize 汇总所有结果为最终输出
func (r *ReviewerAgent) Summarize(ctx context.Context, task string, results []Result) (string, error) {
	prompt := fmt.Sprintf("原始任务：%s\n\n各子任务结果：\n", task)
	for _, res := range results {
		prompt += fmt.Sprintf("【%s】(得分:%d)\n%s\n\n", res.Agent, res.Score, res.Content)
	}
	prompt += "请将以上结果整合为一份完整的最终交付物。"

	return callLLM(r.cfg, "你是一个整合专家，将各子任务结果合并为一份完整、连贯的交付物。", prompt)
}
