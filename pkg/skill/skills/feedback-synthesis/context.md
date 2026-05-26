# {{.ProjectName}} 代码审查反馈综合

## 项目上下文
- **语言**: {{.ProjectLang}}
{{if .ActiveSkills}}
- **当前激活技能**: {{range .ActiveSkills}}- {{.}}{{end}}
{{end}}

## 项目结构
{{.ProjectStructure}}

## 上下文摘要
{{.ContextSummary}}

{{if .Config}}
## 配置
{{range $k, $v := .Config}}- {{$k}}: {{$v}}
{{end}}
{{end}}

---

请根据上述上下文，将代码审查发现汇总成结构化的改进报告。
