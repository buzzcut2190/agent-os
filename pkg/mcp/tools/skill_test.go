package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-os/agent-os/pkg/skill"
	"github.com/stretchr/testify/require"
)

func TestListSkills(t *testing.T) {
	eng := skill.NewEngine("", "")
	err := eng.LoadAll()
	require.NoError(t, err)

	skills := eng.List()
	require.NotEmpty(t, skills, "expected at least one builtin skill")

	// Verify feedback-synthesis is present.
	found := false
	for _, s := range skills {
		if s.Name == "feedback-synthesis" {
			found = true
			require.Equal(t, "builtin", s.Source)
		}
	}
	require.True(t, found, "feedback-synthesis not found")
}

func TestActivateSkill(t *testing.T) {
	eng := skill.NewEngine("", "")
	err := eng.LoadAll()
	require.NoError(t, err)

	// Activate.
	err = eng.Activate("feedback-synthesis")
	require.NoError(t, err)
	require.True(t, eng.IsActive("feedback-synthesis"))

	// Idempotent re-activate.
	err = eng.Activate("feedback-synthesis")
	require.NoError(t, err)

	// Activate nonexistent.
	err = eng.Activate("nonexistent")
	require.Error(t, err)
}

func TestDeactivateSkill(t *testing.T) {
	eng := skill.NewEngine("", "")
	err := eng.LoadAll()
	require.NoError(t, err)

	eng.Activate("feedback-synthesis")
	err = eng.Deactivate("feedback-synthesis")
	require.NoError(t, err)
	require.False(t, eng.IsActive("feedback-synthesis"))

	// Idempotent re-deactivate.
	err = eng.Deactivate("feedback-synthesis")
	require.NoError(t, err)
}

func TestGetSkillContext(t *testing.T) {
	eng := skill.NewEngine("", "test-project")
	eng.SetLangProvider(func() string { return "go" })
	err := eng.LoadAll()
	require.NoError(t, err)

	ctx, err := eng.GetContext("feedback-synthesis")
	require.NoError(t, err)
	require.NotEmpty(t, ctx)
	require.True(t, strings.Contains(ctx, "test-project"), "context should contain project name")
}

func TestGetSkillContextNotFound(t *testing.T) {
	eng := skill.NewEngine("", "")
	eng.LoadAll()

	_, err := eng.GetContext("nonexistent")
	require.Error(t, err)
}

func TestActiveListAfterActivate(t *testing.T) {
	eng := skill.NewEngine("", "")
	eng.LoadAll()

	require.Empty(t, eng.Active(), "no skills should be active initially")

	eng.Activate("feedback-synthesis")
	active := eng.Active()
	require.Len(t, active, 1)
	require.Equal(t, "feedback-synthesis", active[0].Name)

	eng.Activate("code-review")
	active = eng.Active()
	require.Len(t, active, 2)
}

func TestListSkillsJSON(t *testing.T) {
	eng := skill.NewEngine("", "")
	eng.LoadAll()

	skills := eng.List()
	data, err := json.MarshalIndent(skills, "", "  ")
	require.NoError(t, err)

	var parsed []skill.SkillInfo
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	require.NotEmpty(t, parsed)
}

func TestSetSkillEngine(t *testing.T) {
	// Ensure SetSkillEngine works.
	eng := skill.NewEngine("", "")
	eng.LoadAll()
	SetSkillEngine(eng)

	got := getEngine()
	require.NotNil(t, got)
	require.True(t, len(got.List()) > 0)
}

func TestListSkillsFiltered(t *testing.T) {
	eng := skill.NewEngine("", "")
	eng.LoadAll()

	// Filter by active.
	eng.Activate("feedback-synthesis")
	active := eng.Active()
	for _, s := range active {
		require.Equal(t, skill.SkillActive, s.State)
	}

	// Filter by tag.
	all := eng.List()
	var reviewSkills []skill.SkillInfo
	for _, s := range all {
		for _, tag := range s.Tags {
			if tag == "review" {
				reviewSkills = append(reviewSkills, s)
				break
			}
		}
	}
	require.NotEmpty(t, reviewSkills, "should have at least one review-tagged skill")
}
