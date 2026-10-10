// Command movement runs the classic movement module.
package main

import (
	"github.com/agent-wow/agent-wow/modules/classic/internal/modrun"
	"github.com/agent-wow/agent-wow/modules/classic/movement"
	movementv1 "github.com/agent-wow/agent-wow/modules/classic/movement/api"
	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"google.golang.org/grpc"
)

func main() {
	modrun.Main(func(s *grpc.Server, session modv1.SessionClient) error {
		self, err := modrun.CharacterGUID()
		if err != nil {
			return err
		}
		movementv1.RegisterMovementServer(s, movement.New(modrun.Session{Client: session}, self))
		return nil
	})
}
