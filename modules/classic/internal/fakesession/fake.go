// Package fakesession records what a module sends, for tests.
package fakesession

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"
)

// Packet is one recorded client packet.
type Packet struct {
	Op   uint32
	Body []byte
}

// Call is one recorded module invocation.
type Call struct {
	Module, Method string
	Req            proto.Message
}

// Session implements the modules' Session interface in memory.
type Session struct {
	mu      sync.Mutex
	Packets []Packet
	Calls   []Call
	Now     uint32
	// Answer fills the response of an invocation; nil leaves it empty.
	Answer func(module, method string, req, resp proto.Message) error
}

func (s *Session) SendPacket(_ context.Context, op uint32, body []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Packets = append(s.Packets, Packet{op, append([]byte(nil), body...)})
	return nil
}

func (s *Session) Clock(context.Context) (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Now += 10
	return s.Now, nil
}

func (s *Session) Invoke(_ context.Context, module, method string, req, resp proto.Message) error {
	s.mu.Lock()
	s.Calls = append(s.Calls, Call{module, method, proto.Clone(req)})
	answer := s.Answer
	s.mu.Unlock()
	if answer == nil {
		return nil
	}
	if err := answer(module, method, req, resp); err != nil {
		return fmt.Errorf("%s.%s: %w", module, method, err)
	}
	return nil
}

// Sent returns the recorded packets.
func (s *Session) Sent() []Packet {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Packet(nil), s.Packets...)
}
