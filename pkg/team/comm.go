package team

import (
	"errors"
	"fmt"
	"time"
)

// SendMessage delivers a message. When To is "@all" the message is
// broadcast to every registered agent. Message IDs are auto-generated
// when empty.
func (s *TeamStore) SendMessage(msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if msg.ID == "" {
		msg.ID = newUUID()
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	if msg.To == "@all" {
		if len(s.agents) == 0 {
			return nil
		}
		for name := range s.agents {
			cp := msg
			cp.To = name
			s.messages[name] = append(s.messages[name], &cp)
		}
		return nil
	}

	if msg.To == "" {
		return errors.New("message recipient (To) cannot be empty")
	}
	s.messages[msg.To] = append(s.messages[msg.To], &msg)
	return nil
}

// GetInbox returns copies of all messages addressed to agentName.
func (s *TeamStore) GetInbox(agentName string) []*Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msgs := s.messages[agentName]
	result := make([]*Message, len(msgs))
	for i, m := range msgs {
		cp := *m
		cp.Attachments = copySlice(m.Attachments)
		result[i] = &cp
	}
	return result
}

// MarkRead marks a specific message in an agent's inbox as read.
func (s *TeamStore) MarkRead(agentName, msgID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, m := range s.messages[agentName] {
		if m.ID == msgID {
			m.Read = true
			return nil
		}
	}
	return fmt.Errorf("message %s not found in inbox of %s", msgID, agentName)
}

// SendToOutbox appends a message to the sender's outbox.
func (s *TeamStore) SendToOutbox(msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if msg.ID == "" {
		msg.ID = newUUID()
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	if msg.From == "" {
		return errors.New("message sender (From) cannot be empty")
	}

	s.outbox[msg.From] = append(s.outbox[msg.From], &msg)
	return nil
}

// GetOutbox returns copies of all messages sent by agentName.
func (s *TeamStore) GetOutbox(agentName string) []*Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msgs := s.outbox[agentName]
	result := make([]*Message, len(msgs))
	for i, m := range msgs {
		cp := *m
		cp.Attachments = copySlice(m.Attachments)
		result[i] = &cp
	}
	return result
}

// Broadcast sends a message from one agent to every registered agent.
func (s *TeamStore) Broadcast(from, subject, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for name := range s.agents {
		msg := &Message{
			ID:        newUUID(),
			From:      from,
			To:        name,
			Subject:   subject,
			Body:      body,
			Timestamp: time.Now(),
		}
		s.messages[name] = append(s.messages[name], msg)
	}
	return nil
}
