// Command combat runs the classic combat module.
package main

import (
	"github.com/agent-wow/agent-wow/modules/classic/combat"
	combatv1 "github.com/agent-wow/agent-wow/modules/classic/combat/api"
	"github.com/agent-wow/agent-wow/modules/classic/internal/modrun"
	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"google.golang.org/grpc"
)

func main() {
	modrun.Main(func(s *grpc.Server, session modv1.SessionClient) error {
		self, err := modrun.CharacterGUID()
		if err != nil {
			return err
		}
		combatv1.RegisterCombatServer(s, combat.New(modrun.Session{Client: session}, self))
		return nil
	})
}
