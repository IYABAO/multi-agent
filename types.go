package main

import (
	"context"
	"time"
)

// Task 表示一个待执行的子任务
type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	AssignedTo  string `json:"assigned_to,omitempty"`
}

// Result 表示 Worker 的执行结果
type Result struct {
	TaskID    string `json:"task_id"`
	Agent     string `json:"agent"`
	Content   string `json:"content"`
	Score     int    `json:"score"` // 0-10, reviewer 打分
	Approved  bool   `json:"approved"`
	Feedback  string `json:"feedback,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Blackboard 黑板模式：所有 Agent 共享的状态
type Blackboard struct {
	Task   string         `json:"task"`
	Plan   []Task         `json:"plan,omitempty"`
	Results []Result      `json:"results,omitempty"`
	Final  string         `json:"final,omitempty"`
}

// Agent 是所有 Agent 的接口
type Agent interface {
	Name() string
	SystemPrompt() string
	Run(ctx context.Context, input string) (string, error)
}

// Config LLM 配置
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}
