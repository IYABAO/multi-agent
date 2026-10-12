# multi-agent

用 Go 实现的多 Agent 协作编排框架。Planner 拆解任务 → 多专家 Worker 并行执行 → Critic 审查 → Reviewer 评分重试 → 汇总交付。

## 架构

```
用户任务
  ↓
Planner Agent —— LLM 拆解为 N 个子任务，指定专家角色
  ↓
┌─────────────────────────────────────┐
│  第一阶段：普通 Worker 并行执行       │
│  goroutine + WaitGroup               │
│  ├── General-Worker                 │
│  ├── Writer                          │
│  └── Coder                           │
└─────────────────────────────────────┘
  ↓
┌─────────────────────────────────────┐
│  第二阶段：Critic 审查（串行）        │
│  注入其他 Worker 的实际输出作为上下文  │
│  └── Critic                          │
└─────────────────────────────────────┘
  ↓
Reviewer Agent —— LLM 对每个结果打分（1-10）
  ↓
未通过 → 重试（最多 N 次，审查任务注入上下文重试）
  ↓
汇总输出最终交付物
```

## 快速开始

```bash
# 设置 API Key（DeepSeek 兼容 OpenAI 接口）
set DEEPSEEK_API_KEY=sk-xxx

# 编译
go build -o multi-agent.exe .

# 运行
.\multi-agent.exe --task "写一篇关于 Go context 包技术文章"

# 流式输出
.\multi-agent.exe --task "..." --stream

# 自定义重试次数
.\multi-agent.exe --task "..." --retries 3
```

## 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--task` | (必填) | 要执行的复杂任务 |
| `--base` | `https://api.deepseek.com/v1` | LLM API base URL |
| `--model` | `deepseek-chat` | 模型名 |
| `--stream` | `false` | 流式输出 Worker 内容 |
| `--retries` | `2` | 未通过任务最大重试次数 |

## 专家角色

| 角色 | 说明 |
|------|------|
| `general` | 通用执行，兜底角色 |
| `writer` | 技术写作与内容组织 |
| `coder` | 代码实现与技术方案 |
| `critic` | 审查与批判性分析（第二阶段执行，注入其他 Worker 输出） |

## 输出示例

```
🔍 [Planner] 正在拆解任务...
📋 拆解为 4 个子任务：
   [1] 梳理 Go context 包核心概念与用法 → 角色: general
   [2] 编写 Go context 示例代码 → 角色: coder
   [3] 撰写技术介绍文章正文 → 角色: writer
   [4] 审校文章技术准确性与表达 → 角色: critic

⚡ [Worker] 并行执行中...
   ✅ [1] General-Worker (完成)
   ✅ [2] Coder (完成)
   ✅ [3] Writer (完成)
   ✅ [4] Critic (完成)

🔍 [Reviewer] 审查结果...
🔄 有 1 个任务未通过，开始重试...
🔄 [2] 重试第 1 次 (上次得分 6)
   ✅ [1] score=9 feedback=概念框架完整...
   ✅ [2] score=7 feedback=三场景覆盖超时、取消传播...
   ✅ [3] score=8 feedback=叙述流畅、结构合理...
   ✅ [4] score=9 feedback=审校报告分级清晰，逐条定位问题...

📝 [Reviewer] 生成最终交付物...
============================================================
最终交付物：
...
============================================================
统计：4 通过 / 0 未通过 / 4 总任务
```

## 设计要点

- **两阶段执行**：普通任务 goroutine 并行；审查类任务（critic）等所有普通任务完成后串行执行，通过 `RunWithContext` 把其他 Worker 的实际输出注入 prompt，解决"审查者看不到被审内容"的信息隔离问题
- **质量闭环**：Reviewer 输出 NDJSON 逐行评分（score 1-10, ≥7 通过），未通过任务自动重试；审查任务重试时同样注入上下文
- **并发模型**：goroutine + WaitGroup，每个 Worker 独立 system prompt，职责单一
- **流式输出**：SSE 逐 token 打印，支持 `--stream` 参数
- **LLM 兼容**：OpenAI 兼容接口（DeepSeek / 任意 OpenAI-compatible API）

## 项目结构

```
multi-agent/
├── main.go      # 入口：flag 解析、流程编排
├── planner.go   # Planner：LLM 拆解任务为 JSON
├── worker.go    # Worker：多角色池、两阶段执行、重试
├── reviewer.go # Reviewer：NDJSON 评分解析、汇总
├── types.go     # Task / Result / Config 数据结构
└── llm.go       # LLM 调用：非流式 + SSE 流式
```

## License

MIT

## 开源地址

https://github.com/IYABAO/multi-agent
