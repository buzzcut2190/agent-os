# {{.ProjectName}} 自动文档生成

## 项目信息
- **语言**: {{.ProjectLang}}
- **文档目标**: 为公开接口、函数、类型生成规范文档

## 项目结构
{{.ProjectStructure}}

## 涉及文件
{{if .Files}}
{{range .Files}}- `{{.}}`
{{end}}
{{else}}
全部项目文件
{{end}}

## 上下文摘要
{{.ContextSummary}}

{{if .Config}}
## 生成配置
{{range $k, $v := .Config}}- **{{$k}}**: {{$v}}
{{end}}
{{end}}

---

请根据上述上下文，为指定代码生成符合语言习惯的API文档。
