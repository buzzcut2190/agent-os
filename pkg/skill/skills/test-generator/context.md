# {{.ProjectName}} 测试用例生成

## 项目信息
- **语言**: {{.ProjectLang}}
- **目标框架**: {{index .Config "framework"}}
- **覆盖率目标**: {{index .Config "coverage_target"}}%

## 项目结构
{{.ProjectStructure}}

## 上下文摘要
{{.ContextSummary}}

## 目标文件
{{if .Files}}
{{range .Files}}- `{{.}}`
{{end}}
{{else}}
未指定，将针对所有未测试的公开接口生成测试。
{{end}}

## 代码差异
{{if .Diff}}
```
{{.Diff}}
```
{{end}}

---

请根据上述上下文，为目标代码生成全面的测试用例，确保测试覆盖正常路径、边界条件和错误路径。
