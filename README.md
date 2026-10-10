# multi-agent

用 Go 实现的多 Agent 协作编排框架。Planner 拆解任务 → Worker 并行执行 → Reviewer 审查汇总。

## 架构

```
用户任务
  ↓
Planner Agent —— 拆解为 N 个子任务
  ↓
Worker Agent × N —— goroutine 并行执行
  ↓
Reviewer Agent —— 审查结果打分
  ↓
汇总输出最终交付物
```

## 快速开始

```bash
# 设置 API Key
set DEEPSEEK_API_KEY=sk-xxx

# 运行
.\multi-agent.exe --task "写一篇关于 Go context 包技术文章，包含原理、用法和最佳实践"
```

## 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| --task | (必填) | 要执行的复杂任务 |
| --base | https://api.deepseek.com/v1 | LLM API 地址 |
| --model | deepseek-chat | 模型名 |

## 输出示例

```
🔍 [Planner] 正在拆解任务...
📋 拆解为 3 个子任务：
   [1] 调研 Go context 原理
   [2] 编写代码示例
   [3] 整理最佳实践

⚡ [Worker] 并行执行中...
   ✅ [1] General-Worker (完成)
   ✅ [2] General-Worker (完成)
   ✅ [3] General-Worker (完成)

🔍 [Reviewer] 审查结果...
   ✅ [1] score=9 feedback=原理讲解清晰
   ✅ [2] score=8 feedback=示例完整
   ✅ [3] score=8 feedback=实用

📝 [Reviewer] 生成最终交付物...

============================================================
最终交付物：
...
============================================================
```

## 设计要点

- **并发模型**：goroutine + WaitGroup 实现 Worker 并行
- **黑板模式**：Blackboard 结构体在 Agent 间传递任务和结果
- **质量闭环**：Reviewer 审查打分，未通过的任务可标记重试
- **LLM 编排**：每个 Agent 有独立 system prompt，职责单一

## 开源地址

https://github.com/IYABAO/multi-agent
