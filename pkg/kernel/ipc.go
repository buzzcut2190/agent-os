package kernel

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Message is an IPC message sent between agents.
type Message struct {
	From    AgentID   `json:"from"`
	To      AgentID   `json:"to"`
	Channel string    `json:"channel"`
	Subject string    `json:"subject"`
	Body    string    `json:"body"`
	ID      string    `json:"id"`
	Time    time.Time `json:"time"`
}

// IPC provides inter-agent communication via channels.
type IPC struct {
	mu          sync.RWMutex
	subscribers map[string]map[AgentID]chan Message // channel → agent → chan
}

// NewIPC creates a new IPC system.
func NewIPC() *IPC {
	return &IPC{
		subscribers: make(map[string]map[AgentID]chan Message),
	}
}

// Send delivers a message to a specific agent. The agent must be subscribed
// to the given channel.
func (ipc *IPC) Send(from, to AgentID, channel string, subject, body string) error {
	if from == "" || to == "" {
		return fmt.Errorf("from and to are required")
	}
	msg := Message{
		From:    from,
		To:      to,
		Channel: channel,
		Subject: subject,
		Body:    body,
		ID:      uuid.New().String()[:8],
		Time:    time.Now(),
	}
	ipc.mu.RLock()
	defer ipc.mu.RUnlock()
	agents, ok := ipc.subscribers[channel]
	if !ok {
		return fmt.Errorf("channel %s has no subscribers", channel)
	}
	ch, ok := agents[to]
	if !ok {
		return fmt.Errorf("agent %s not subscribed to channel %s", to, channel)
	}
	select {
	case ch <- msg:
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("send timeout")
	}
}

// Broadcast sends a message to all subscribers of a channel.
func (ipc *IPC) Broadcast(sender AgentID, channel string, subject, body string) []AgentID {
	ipc.mu.RLock()
	defer ipc.mu.RUnlock()
	agents, ok := ipc.subscribers[channel]
	if !ok {
		return nil
	}
	var delivered []AgentID
	for id, ch := range agents {
		if id == sender {
			continue
		}
		msg := Message{
			From:    sender,
			To:      id,
			Channel: channel,
			Subject: subject,
			Body:    body,
			ID:      uuid.New().String()[:8],
			Time:    time.Now(),
		}
		select {
		case ch <- msg:
			delivered = append(delivered, id)
		default:
		}
	}
	return delivered
}

// Subscribe registers an agent to receive messages on a channel.
func (ipc *IPC) Subscribe(agentID AgentID, channel string) chan Message {
	ipc.mu.Lock()
	defer ipc.mu.Unlock()
	if _, ok := ipc.subscribers[channel]; !ok {
		ipc.subscribers[channel] = make(map[AgentID]chan Message)
	}
	ch := make(chan Message, 64)
	ipc.subscribers[channel][agentID] = ch
	return ch
}

// Unsubscribe removes an agent from a channel.
func (ipc *IPC) Unsubscribe(agentID AgentID, channel string) {
	ipc.mu.Lock()
	defer ipc.mu.Unlock()
	if agents, ok := ipc.subscribers[channel]; ok {
		if ch, ok2 := agents[agentID]; ok2 {
			close(ch)
			delete(agents, agentID)
		}
	}
}

// Channels returns the names of all active channels.
func (ipc *IPC) Channels() []string {
	ipc.mu.RLock()
	defer ipc.mu.RUnlock()
	names := make([]string, 0, len(ipc.subscribers))
	for n := range ipc.subscribers {
		names = append(names, n)
	}
	return names
}
