package main

import (
	"context"
	"encoding/json"
	"fmt"
)

// PlannerAgent 负责拆解任务
type PlannerAgent struct {
	cfg *Config
}

func (p *PlannerAgent) Name() string { return "Planner" }

func (p *PlannerAgent) SystemPrompt() string {
	return `你是一个任务规划专家。将用户的复杂任务拆解为2-4个可独立执行的子任务。
输出严格为 JSON 数组，每个元素格式：
{"id":"1","title":"子任务标题","description":"具体要求"}
不要输出其他内容。`
}

func (p *PlannerAgent) Run(ctx context.Context, task string) ([]Task, error) {
	prompt := fmt.Sprintf("请拆解以下任务：\n%s", task)
	resp, err := callLLM(p.cfg, p.SystemPrompt(), prompt)
	if err != nil {
		return nil, err
	}

	var tasks []Task
	if err := json.Unmarshal([]byte(resp), &tasks); err != nil {
		return nil, fmt.Errorf("planner parse error: %w, raw: %s", err, resp)
	}
	return tasks, nil
}
