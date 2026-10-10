// Command world runs the classic world-state module.
package main

import (
	"github.com/agent-wow/agent-wow/modules/classic/internal/modrun"
	"github.com/agent-wow/agent-wow/modules/classic/world"
	worldv1 "github.com/agent-wow/agent-wow/modules/classic/world/api"
	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"google.golang.org/grpc"
)

func main() {
	modrun.Main(func(s *grpc.Server, _ modv1.SessionClient) error {
		self, err := modrun.CharacterGUID()
		if err != nil {
			return err
		}
		worldv1.RegisterWorldServer(s, world.New(self))
		return nil
	})
}
