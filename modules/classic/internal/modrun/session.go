package modrun

import (
	"context"
	"fmt"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Session adapts the agent-wow session API for the module services.
type Session struct{ Client modv1.SessionClient }

// SendPacket writes one client packet. Success confirms the write only.
func (s Session) SendPacket(ctx context.Context, op uint32, body []byte) error {
	_, err := s.Client.SendPacket(ctx, &modv1.SendPacketRequest{Opcode: op, Payload: body})
	return err
}

// Clock returns the session clock in milliseconds, the client time of
// movement packets.
func (s Session) Clock(ctx context.Context) (uint32, error) {
	r, err := s.Client.GetClock(ctx, &emptypb.Empty{})
	if err != nil {
		return 0, err
	}
	return r.GetClientTimeMs(), nil
}

// Invoke calls a method alias on a module this one requires.
func (s Session) Invoke(ctx context.Context, module, method string, req, resp proto.Message) error {
	packed, err := anypb.New(req)
	if err != nil {
		return err
	}
	r, err := s.Client.InvokeModule(ctx, &modv1.InvokeModuleRequest{Module: module, Method: method, Request: packed})
	if err != nil {
		return fmt.Errorf("%s.%s: %w", module, method, err)
	}
	return r.GetResult().UnmarshalTo(resp)
}
