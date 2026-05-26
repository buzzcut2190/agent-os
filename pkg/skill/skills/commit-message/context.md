# {{.ProjectName}} 提交信息生成

## 项目信息
- **语言**: {{.ProjectLang}}
- **提交风格**: {{index .Config "style"}}
- **标题最大长度**: {{index .Config "max_length"}} 字符

## 变更摘要
{{.ContextSummary}}

## 代码差异
{{if .Diff}}
```
{{.Diff}}
```
{{else}}
无变更内容
{{end}}

{{if .Files}}
## 涉及文件
{{range .Files}}- `{{.}}`
{{end}}
{{end}}

---

请基于上述变更内容，生成符合 Conventional Commits 规范的提交信息。
