// Command time runs the classic time-query example module.
package main

import (
	"github.com/agent-wow/agent-wow/modules/classic/internal/modrun"
	classictime "github.com/agent-wow/agent-wow/modules/classic/time"
	timev1 "github.com/agent-wow/agent-wow/modules/classic/time/api"
	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"google.golang.org/grpc"
)

func main() {
	modrun.Main(func(s *grpc.Server, session modv1.SessionClient) error {
		timev1.RegisterTimeServer(s, classictime.New(modrun.Session{Client: session}))
		return nil
	})
}
