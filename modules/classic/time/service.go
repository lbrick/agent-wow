// Package time is go-module-template's example ported to WoW 1.12.1, where
// SMSG_QUERY_TIME_RESPONSE carries only a uint32 Unix time
// (mangos-classic WorldSession::SendQueryTimeResponse).
package time

import (
	"context"
	"encoding/binary"
	"sync"

	"github.com/agent-wow/agent-wow/modules/classic/internal/modrun"
	timev1 "github.com/agent-wow/agent-wow/modules/classic/time/api"
	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode/classic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Service implements timev1.TimeServer.
type Service struct {
	timev1.UnimplementedTimeServer
	session modrun.Session

	mu         sync.Mutex
	packets    uint32
	serverTime *uint32
}

func New(session modrun.Session) *Service { return &Service{session: session} }

func (s *Service) Status(ctx context.Context, _ *emptypb.Empty) (*timev1.StatusResponse, error) {
	now, err := s.session.Clock(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return &timev1.StatusResponse{ClientTimeMs: now, PacketsReceived: s.packets, ServerTimeUnix: s.serverTime}, nil
}

func (s *Service) QueryTime(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, s.session.SendPacket(ctx, classic.CMSGQueryTime, nil)
}

func (s *Service) OnPacket(_ context.Context, p *modv1.WorldPacket) (*emptypb.Empty, error) {
	if p.GetOpcode() != classic.SMSGQueryTimeResponse || len(p.GetPayload()) != 4 {
		return nil, status.Error(codes.InvalidArgument, "expected a four-byte SMSG_QUERY_TIME_RESPONSE")
	}
	t := binary.LittleEndian.Uint32(p.GetPayload())
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packets++
	s.serverTime = &t
	return &emptypb.Empty{}, nil
}

func (s *Service) BeforeLogout(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
