package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TeamWorkspace extends a base Workspace with team‑collaboration
// facilities: member management, shared context, decisions, and
// broadcast messaging via a simple channel (intended to be bridged to
// the kernel IPC layer).
type TeamWorkspace struct {
	ws        *Workspace
	root      string

	mu         sync.RWMutex
	members    map[AgentID]bool
	context    *SharedContext
	decisions  []*Decision
	broadcasts chan BroadcastMessage
}

// BroadcastMessage carries a message sent to all team members.
type BroadcastMessage struct {
	From      AgentID   `json:"from"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Timestamp time.Time `json:"timestamp"`
}

// NewTeamWorkspace wraps an existing workspace with team functionality.
// The workspace must have Type == TypeTeam.
func NewTeamWorkspace(ws *Workspace, root string) *TeamWorkspace {
	tw := &TeamWorkspace{
		ws:         ws,
		root:       root,
		members:    make(map[AgentID]bool),
		context:    &SharedContext{},
		broadcasts: make(chan BroadcastMessage, 64),
	}
	tw.loadState()
	return tw
}

// BroadcastChannel returns the read‑only broadcast channel that the
// kernel IPC layer should consume.
func (tw *TeamWorkspace) BroadcastChannel() <-chan BroadcastMessage {
	return tw.broadcasts
}

// AddMember registers an agent as a team member. Idempotent.
func (tw *TeamWorkspace) AddMember(agentID AgentID) error {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	tw.members[agentID] = true
	return tw.persistMembers()
}

// RemoveMember unregisters an agent from the team. Idempotent.
func (tw *TeamWorkspace) RemoveMember(agentID AgentID) error {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	delete(tw.members, agentID)
	return tw.persistMembers()
}

// Members returns a copy of the current member set.
func (tw *TeamWorkspace) Members() []AgentID {
	tw.mu.RLock()
	defer tw.mu.RUnlock()

	list := make([]AgentID, 0, len(tw.members))
	for id := range tw.members {
		list = append(list, id)
	}
	sort.Slice(list, func(i, j int) bool {
		return string(list[i]) < string(list[j])
	})
	return list
}

// Broadcast sends a message to the broadcast channel. The kernel IPC
// layer is expected to consume messages from BroadcastChannel() and
// deliver them to member agents.
func (tw *TeamWorkspace) Broadcast(from AgentID, subject, body string) {
	msg := BroadcastMessage{
		From:      from,
		Subject:   subject,
		Body:      body,
		Timestamp: time.Now(),
	}

	select {
	case tw.broadcasts <- msg:
	default:
		// Drop if channel is full — production use would wire a
		// proper IPC transport.
	}
}

// GetSharedContext returns a copy of the team's shared context.
func (tw *TeamWorkspace) GetSharedContext() *SharedContext {
	tw.mu.RLock()
	defer tw.mu.RUnlock()

	cp := *tw.context
	cp.Conventions = copyStringSlice(tw.context.Conventions)
	cp.ActiveBranches = copyStringSlice(tw.context.ActiveBranches)
	return &cp
}

// SetSharedContext updates the team's shared context and persists it.
func (tw *TeamWorkspace) SetSharedContext(ctx *SharedContext) error {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	tw.context = ctx
	return tw.persistContext()
}

// AddDecision records a new team decision and persists it.
func (tw *TeamWorkspace) AddDecision(dec Decision) (*Decision, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if dec.ID == "" {
		dec.ID = uuid.New().String()
	}
	if dec.Timestamp.IsZero() {
		dec.Timestamp = time.Now()
	}
	if dec.Status == "" {
		dec.Status = DecisionActive
	}

	tw.decisions = append(tw.decisions, &dec)
	if err := tw.persistDecisions(); err != nil {
		return nil, err
	}
	cp := dec
	return &cp, nil
}

// GetDecisions returns a copy of all recorded decisions.
func (tw *TeamWorkspace) GetDecisions() []*Decision {
	tw.mu.RLock()
	defer tw.mu.RUnlock()

	result := make([]*Decision, len(tw.decisions))
	for i, d := range tw.decisions {
		cp := *d
		result[i] = &cp
	}
	return result
}

// updateDecisionStatus changes the status of a decision. It returns an
// error if the decision ID is not found.
func (tw *TeamWorkspace) updateDecisionStatus(id string, status DecisionStatus) error {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	for _, d := range tw.decisions {
		if d.ID == id {
			d.Status = status
			return tw.persistDecisions()
		}
	}
	return fmt.Errorf("team: decision %s not found", id)
}

// SupersedeDecision marks a decision as superseded.
func (tw *TeamWorkspace) SupersedeDecision(id string) error {
	return tw.updateDecisionStatus(id, DecisionSuperseded)
}

// RevertDecision marks a decision as reverted.
func (tw *TeamWorkspace) RevertDecision(id string) error {
	return tw.updateDecisionStatus(id, DecisionReverted)
}

// ---------------------------------------------------------------------------
// Persistence helpers
// ---------------------------------------------------------------------------

func (tw *TeamWorkspace) membersPath() string {
	return filepath.Join(tw.root, "team_members.json")
}

func (tw *TeamWorkspace) contextPath() string {
	return filepath.Join(tw.root, "team_context.json")
}

func (tw *TeamWorkspace) decisionsPath() string {
	return filepath.Join(tw.root, "team_decisions.json")
}

func (tw *TeamWorkspace) persistMembers() error {
	ids := make([]AgentID, 0, len(tw.members))
	for id := range tw.members {
		ids = append(ids, id)
	}
	data, err := json.MarshalIndent(ids, "", "  ")
	if err != nil {
		return fmt.Errorf("team: marshal members: %w", err)
	}
	return os.WriteFile(tw.membersPath(), data, 0o644)
}

func (tw *TeamWorkspace) persistContext() error {
	data, err := json.MarshalIndent(tw.context, "", "  ")
	if err != nil {
		return fmt.Errorf("team: marshal context: %w", err)
	}
	return os.WriteFile(tw.contextPath(), data, 0o644)
}

func (tw *TeamWorkspace) persistDecisions() error {
	data, err := json.MarshalIndent(tw.decisions, "", "  ")
	if err != nil {
		return fmt.Errorf("team: marshal decisions: %w", err)
	}
	return os.WriteFile(tw.decisionsPath(), data, 0o644)
}

func (tw *TeamWorkspace) loadState() {
	// Load members.
	if data, err := os.ReadFile(tw.membersPath()); err == nil {
		var ids []AgentID
		if json.Unmarshal(data, &ids) == nil {
			for _, id := range ids {
				tw.members[id] = true
			}
		}
	}

	// Load shared context.
	if data, err := os.ReadFile(tw.contextPath()); err == nil {
		var ctx SharedContext
		if json.Unmarshal(data, &ctx) == nil {
			tw.context = &ctx
		}
	}

	// Load decisions.
	if data, err := os.ReadFile(tw.decisionsPath()); err == nil {
		var decs []*Decision
		if json.Unmarshal(data, &decs) == nil {
			tw.decisions = decs
		}
	}
}

func copyStringSlice(src []string) []string {
	if src == nil {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}
