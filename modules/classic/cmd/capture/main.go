// Command capture logs a build-5875 character in without modules, saves the
// gameplay packets it receives for a few seconds and reports what package wire
// decodes from them. It is a development aid for the classic modules: run it
// against a realm you may log in to, never with a character another session
// is playing.
//
//	go run ./modules/classic/cmd/capture --config <config.yaml> --guid <guid> --out <dir> [--seconds 20]
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/agent-wow/agent-wow/internal/config"
	"github.com/agent-wow/agent-wow/internal/credentials"
	"github.com/agent-wow/agent-wow/modules/classic/internal/wire"
	"github.com/agent-wow/agent-wow/pkg/auth"
	"github.com/agent-wow/agent-wow/pkg/opcode/classic"
	"github.com/agent-wow/agent-wow/pkg/protocol"
	"github.com/agent-wow/agent-wow/pkg/worldconn"
)

func main() {
	configFile := flag.String("config", "", "agent-wow config with client_build 5875")
	guid := flag.Uint64("guid", 0, "character GUID to log in")
	out := flag.String("out", "", "directory for captured packet bodies")
	seconds := flag.Int("seconds", 20, "capture duration")
	flag.Parse()
	if err := run(*configFile, *guid, *out, time.Duration(*seconds)*time.Second); err != nil {
		log.Fatal(err)
	}
}

func run(configFile string, guid uint64, out string, duration time.Duration) error {
	if err := config.Init(configFile); err != nil {
		return err
	}
	cfg := config.Get()
	if cfg.Profile() != protocol.Classic || guid == 0 || out == "" {
		return fmt.Errorf("need client_build 5875, --guid and --out")
	}
	creds, err := credentials.Read(cfg.AuthFilePath)
	if err != nil {
		return err
	}
	client, err := auth.NewClient(net.JoinHostPort(cfg.AuthServer.Host, strconv.Itoa(cfg.AuthServer.Port)), protocol.Classic)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	session, realms, err := client.LoginRealms(ctx, creds.Username, creds.Password)
	if err != nil || len(realms) == 0 {
		return fmt.Errorf("login: %v (%d realms)", err, len(realms))
	}
	conn, err := worldconn.Dial(ctx, realms[0], session)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.WritePacket(classic.CMSGPlayerLogin, binary.LittleEndian.AppendUint64(nil, guid)); err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := conn.SetDeadline(time.Now().Add(duration)); err != nil {
		return err
	}
	counts := map[string]int{}
	n := 0
	for {
		op, body, err := conn.ReadPacket()
		if err != nil {
			break // deadline
		}
		name := classic.WorldName(uint32(op))
		counts[name]++
		switch op {
		case classic.SMSGUpdateObject, classic.SMSGCompressedUpdateObject, classic.SMSGMonsterMove:
			n++
			if err := os.WriteFile(filepath.Join(out, fmt.Sprintf("%04d_%s.bin", n, name)), body, 0o644); err != nil {
				return err
			}
			report(op, body)
		}
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	_, _ = conn.WritePacket(classic.CMSGLogoutRequest, nil)
	fmt.Println("packet counts:", counts)
	return nil
}

func report(op uint16, body []byte) {
	if op == classic.SMSGMonsterMove {
		m, err := wire.ParseMonsterMove(body)
		fmt.Printf("MONSTER_MOVE %#x stop=%t dest=%+v err=%v\n", m.GUID, m.Stop, m.Destination, err)
		return
	}
	if op == classic.SMSGCompressedUpdateObject {
		var err error
		if body, err = wire.Inflate(body); err != nil {
			fmt.Println("inflate:", err)
			return
		}
	}
	blocks, err := wire.ParseUpdate(body)
	units := 0
	for _, b := range blocks {
		if b.TypeID == wire.TypeUnit || b.TypeID == wire.TypePlayer {
			units++
		}
	}
	fmt.Printf("UPDATE %d bytes: %d blocks, %d units/players, err=%v\n", len(body), len(blocks), units, err)
	for _, b := range blocks {
		if b.TypeID == wire.TypePlayer && b.Position != nil {
			fmt.Printf("  player %#x at %+v level=%d hp=%d/%d xp=%d/%d faction=%d run=%.2f\n", b.GUID, *b.Position,
				b.Values[wire.FieldLevel], b.Values[wire.FieldHealth], b.Values[wire.FieldMaxHealth],
				b.Values[wire.FieldPlayerXP], b.Values[wire.FieldPlayerNextXP], b.Values[wire.FieldFactionTemplate], b.RunSpeed)
		}
		if b.TypeID == wire.TypeUnit && b.Position != nil {
			fmt.Printf("  unit %#x entry=%d level=%d hp=%d/%d faction=%d at (%.1f,%.1f,%.1f)\n", b.GUID, b.Values[wire.FieldEntry],
				b.Values[wire.FieldLevel], b.Values[wire.FieldHealth], b.Values[wire.FieldMaxHealth], b.Values[wire.FieldFactionTemplate],
				b.Position.X, b.Position.Y, b.Position.Z)
		}
	}
}
