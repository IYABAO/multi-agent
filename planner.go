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
每个子任务指定最合适的专家角色（assigned_to），从以下选择：
- "writer"：需要写作、内容组织、文案的子任务
- "coder"：需要写代码、技术实现、方案设计的子任务
- "critic"：需要审查、分析、批判性评估的子任务
- "general"：通用任务

输出严格为 JSON 数组，每个元素格式：
{"id":"1","title":"子任务标题","description":"具体要求","assigned_to":"writer"}
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
