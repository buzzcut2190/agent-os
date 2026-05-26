# {{.ProjectName}} 依赖审计

## 项目信息
- **语言**: {{.ProjectLang}}
- **CVE 检查**: {{index .Config "check_cves"}}
- **更新检查**: {{index .Config "check_updates"}}

## 项目结构
{{.ProjectStructure}}

## 上下文摘要
{{.ContextSummary}}

## 依赖清单
{{if .Files}}
{{range .Files}}- `{{.}}`
{{end}}
{{else}}
请分析项目中所有依赖声明文件 (go.mod, requirements.txt, package.json, Cargo.toml 等)
{{end}}

{{if .Config}}
## 审计配置
{{range $k, $v := .Config}}- **{{$k}}**: {{$v}}
{{end}}
{{end}}

---

请对项目依赖进行全面审计，识别安全漏洞、过期版本和不必要的依赖。
