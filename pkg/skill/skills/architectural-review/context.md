# {{.ProjectName}} 架构审查

## 项目信息
- **语言**: {{.ProjectLang}}
- **分析深度**: {{index .Config "max_depth"}} 层
- **循环依赖检测**: {{index .Config "check_cycles"}}

## 项目结构
{{.ProjectStructure}}

## 上下文摘要
{{.ContextSummary}}

{{if .ActiveSkills}}
## 当前激活技能
{{range .ActiveSkills}}- {{.}}
{{end}}
{{end}}

## 涉及文件
{{if .Files}}
{{range .Files}}- `{{.}}`
{{end}}
{{end}}

---

请对上述项目进行全面的架构审查，分析模块依赖关系、识别架构异味，并提供可操作的改进建议。
