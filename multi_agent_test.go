package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSelectWorker(t *testing.T) {
	cfg := &Config{}
	pool := DefaultWorkerPool(cfg)

	tests := []struct {
		name     string
		task     Task
		wantRole string
	}{
		{"critic映射", Task{ID: "4", AssignedTo: "critic"}, "Critic"},
		{"writer映射", Task{ID: "2", AssignedTo: "writer"}, "Writer"},
		{"coder映射", Task{ID: "3", AssignedTo: "coder"}, "Coder"},
		{"空角色fallback到general", Task{ID: "1", AssignedTo: ""}, "General-Worker"},
		{"未知角色fallback到general", Task{ID: "5", AssignedTo: "unknown"}, "General-Worker"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := selectWorker(pool, tt.task)
			if w.Name() != tt.wantRole {
				t.Errorf("selectWorker() = %s, want %s", w.Name(), tt.wantRole)
			}
		})
	}
}

func TestIsReviewTask(t *testing.T) {
	tests := []struct {
		name string
		task Task
		want bool
	}{
		{"assigned_to=critic", Task{AssignedTo: "critic"}, true},
		{"assigned_to=reviewer", Task{AssignedTo: "reviewer"}, true},
		{"标题含审查", Task{Title: "审查文章技术准确性"}, true},
		{"描述含评估", Task{Description: "评估方案合理性"}, true},
		{"标题含英文review", Task{Title: "Review the code"}, true},
		{"普通writer任务", Task{AssignedTo: "writer", Title: "撰写文章"}, false},
		{"普通coder任务", Task{AssignedTo: "coder", Title: "编写示例代码"}, false},
		{"普通general任务", Task{AssignedTo: "general", Title: "梳理知识点"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isReviewTask(tt.task); got != tt.want {
				t.Errorf("isReviewTask() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseReviewNDJSON(t *testing.T) {
	input := `{"task_id":"1","score":9,"approved":true,"feedback":"结构完整"}
{"task_id":"2","score":6,"approved":false,"feedback":"缺少错误处理"}
some random text
{"task_id":"3","score":8,"approved":true,"feedback":"代码规范"}`

	result := parseReviewNDJSON(input)

	if len(result) != 3 {
		t.Fatalf("expected 3 reviews, got %d", len(result))
	}

	r1 := result["1"]
	if r1.Score != 9 || !r1.Approved || r1.Feedback != "结构完整" {
		t.Errorf("task1 = %+v", r1)
	}

	r2 := result["2"]
	if r2.Score != 6 || r2.Approved {
		t.Errorf("task2 should be disapproved, got %+v", r2)
	}

	r3 := result["3"]
	if r3.Score != 8 {
		t.Errorf("task3 score = %d, want 8", r3.Score)
	}
}

func TestParseReviewNDJSON_Empty(t *testing.T) {
	result := parseReviewNDJSON("no json here\njust text")
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestParseReviewNDJSON_MalformedLines(t *testing.T) {
	input := `{"task_id":"1","score":9,"approved":true}
{invalid json}
{"task_id":"2","score":7,"approved":true,"feedback":"ok"}`

	result := parseReviewNDJSON(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 valid reviews, got %d", len(result))
	}
	if result["1"].Score != 9 || result["2"].Score != 7 {
		t.Errorf("parse result = %+v", result)
	}
}

func TestParseReviewNDJSON_MultilineObjects(t *testing.T) {
	// 模拟真实场景：Reviewer 输出了多个 JSON 对象，可能带前缀文字
	input := `好的，以下是审查结果：
{"task_id":"1","score":9,"approved":true,"feedback":"质量高"}
{"task_id":"2","score":5,"approved":false,"feedback":"需要补充"}`

	result := parseReviewNDJSON(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 reviews, got %d", len(result))
	}
	if result["1"].Feedback != "质量高" {
		t.Errorf("task1 feedback = %q", result["1"].Feedback)
	}
}

func TestParsePlannerTasks(t *testing.T) {
	tests := []struct {
		name    string
		resp    string
		want    int
		wantErr bool
	}{
		{
			name: "纯 JSON 数组",
			resp: `[{"id":"1","title":"梳理概念","assigned_to":"general"}]`,
			want: 1,
		},
		{
			name: "带 json 代码围栏",
			resp: "```json\n[{\"id\":\"1\",\"title\":\"写代码\",\"assigned_to\":\"coder\"}]\n```",
			want: 1,
		},
		{
			name: "带前置说明",
			resp: "好的，拆解如下：\n[{\"id\":\"1\",\"title\":\"审查\",\"assigned_to\":\"critic\"}]\n请执行。",
			want: 1,
		},
		{
			name:    "无 JSON",
			resp:    "抱歉，我无法拆解这个任务。",
			wantErr: true,
		},
		{
			name:    "空数组",
			resp:    `[]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks, err := parsePlannerTasks(tt.resp)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %d tasks", len(tasks))
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePlannerTasks error: %v", err)
			}
			if len(tasks) != tt.want {
				t.Fatalf("got %d tasks, want %d", len(tasks), tt.want)
			}
		})
	}
}

func TestPlannerRunAcceptsFencedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("request path = %q, want /chat/completions", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{
					"content": "```json\n[{\"id\":\"1\",\"title\":\"审查\",\"assigned_to\":\"critic\"}]\n```",
				}},
			},
		})
	}))
	defer server.Close()

	p := &PlannerAgent{cfg: &Config{BaseURL: server.URL, APIKey: "test", Model: "test"}}
	tasks, err := p.Run(context.Background(), "测试任务")
	if err != nil {
		t.Fatalf("PlannerAgent.Run error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if tasks[0].AssignedTo != "critic" {
		t.Errorf("assigned_to = %q, want critic", tasks[0].AssignedTo)
	}
}
