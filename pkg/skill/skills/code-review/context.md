# {{.ProjectName}} 代码审查上下文

## 项目信息
- **语言**: {{.ProjectLang}}
- **审查目标**: 安全检查、性能瓶颈、代码风格、最佳实践

## 项目结构
{{.ProjectStructure}}

## 变更范围
{{if .Diff}}
```
{{.Diff}}
```
{{end}}

## 上下文摘要
{{.ContextSummary}}

{{if .ActiveSkills}}
## 当前激活技能
{{range .ActiveSkills}}- {{.}}
{{end}}
{{end}}

{{if .Config}}
## 审查配置
{{range $k, $v := .Config}}- **{{$k}}**: {{$v}}
{{end}}
{{end}}

---

请根据上述上下文对代码变更进行系统化审查，涵盖安全性、性能、可维护性和代码规范。
