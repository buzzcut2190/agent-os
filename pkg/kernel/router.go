package kernel

// RoutingRule maps task types to specific models based on complexity.
type RoutingRule struct {
	TaskType      AgentType `yaml:"task_type" json:"task_type"`
	MinComplexity int       `yaml:"min_complexity" json:"min_complexity"`
	Model         string    `yaml:"model" json:"model"`
	Provider      string    `yaml:"provider" json:"provider"`
}

// DefaultRoutingRules returns sensible routing defaults.
func DefaultRoutingRules() []RoutingRule {
	return []RoutingRule{
		{TaskType: AgentTypeCodeReview, MinComplexity: 0, Model: "deepseek-chat", Provider: "deepseek"},
		{TaskType: AgentTypeTester, MinComplexity: 0, Model: "deepseek-chat", Provider: "deepseek"},
		{TaskType: AgentTypeDeveloper, MinComplexity: 0, Model: "deepseek-chat", Provider: "deepseek"},
		{TaskType: AgentTypeArchitect, MinComplexity: 5, Model: "deepseek-reasoner", Provider: "deepseek"},
		{TaskType: AgentTypeOrchestrator, MinComplexity: 5, Model: "deepseek-reasoner", Provider: "deepseek"},
		{TaskType: AgentTypeReporter, MinComplexity: 0, Model: "deepseek-chat", Provider: "deepseek"},
		{TaskType: AgentTypeMonitor, MinComplexity: 0, Model: "deepseek-chat", Provider: "deepseek"},
	}
}

// ModelRouter selects the appropriate model for a task.
type ModelRouter struct {
	rules []RoutingRule
}

// NewModelRouter creates a router from rules.
func NewModelRouter(rules []RoutingRule) *ModelRouter {
	return &ModelRouter{rules: rules}
}

// Route selects a model and provider for a task.
func (mr *ModelRouter) Route(task Task) (model, provider string) {
	complexity := task.Priority // approximate
	for _, rule := range mr.rules {
		if rule.TaskType == task.AgentType && complexity >= rule.MinComplexity {
			return rule.Model, rule.Provider
		}
	}
	return "deepseek-chat", "deepseek"
}
