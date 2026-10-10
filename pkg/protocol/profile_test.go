package protocol

import (
	"strings"
	"testing"

	"github.com/agent-wow/agent-wow/pkg/opcode"
)

func TestForBuild(t *testing.T) {
	if p, err := ForBuild(12340); err != nil || p != Wrath {
		t.Fatalf("ForBuild(12340) = %v, %v", p, err)
	}
	if p, err := ForBuild(DefaultBuild); err != nil || p.Build != DefaultBuild {
		t.Fatalf("default build = %v, %v", p, err)
	}
	if _, err := ForBuild(8606); err == nil || !strings.Contains(err.Error(), "supported builds") {
		t.Fatalf("unsupported build error = %v", err)
	}
}

func TestWrathCatalog(t *testing.T) {
	if Wrath.WorldName(opcode.SMSGUpdateObject) != "SMSG_UPDATE_OBJECT" || Wrath.WorldName(1<<20) != "UNKNOWN" {
		t.Fatal("unexpected world names")
	}
	if op, ok := Wrath.WorldOpcode("SMSG_UPDATE_OBJECT"); !ok || op != opcode.SMSGUpdateObject {
		t.Fatalf("WorldOpcode = %#x, %t", op, ok)
	}
	if _, ok := Wrath.WorldOpcode("SMSG_NOT_A_PACKET"); ok {
		t.Fatal("resolved an unknown label")
	}
	for _, op := range []uint32{opcode.CMSGAuthSession, opcode.CMSGPing, opcode.CMSGTimeSyncResp, opcode.CMSGLogoutRequest, opcode.CMSGWardenData} {
		if !Wrath.Reserved(op) {
			t.Errorf("%s is not reserved", Wrath.WorldName(op))
		}
	}
	if Wrath.Reserved(opcode.CMSGAttackswing) {
		t.Error("gameplay opcode reserved")
	}
}

func TestClassicCatalog(t *testing.T) {
	p, err := ForBuild(5875)
	if err != nil || p != Classic || p.TimeSync || p.Warden || p.Version != [3]byte{1, 12, 1} {
		t.Fatalf("ForBuild(5875) = %+v, %v", p, err)
	}
	// Values from mangos-classic Opcodes.h at 8ec338a1.
	if p.Op.CMSGAuthSession != 0x1ed || p.Op.SMSGCharEnum != 0x03b || p.Op.SMSGLoginVerifyWorld != 0x236 {
		t.Fatalf("unexpected opcodes %+v", p.Op)
	}
	if op, ok := p.WorldOpcode("SMSG_UPDATE_OBJECT"); !ok || p.WorldName(uint32(op)) != "SMSG_UPDATE_OBJECT" {
		t.Fatal("SMSG_UPDATE_OBJECT does not round-trip")
	}
	if _, ok := p.WorldOpcode("SMSG_TIME_SYNC_REQ"); ok {
		t.Fatal("1.12 resolved a 3.3.5a-only opcode")
	}
	if !p.Reserved(uint32(p.Op.CMSGPing)) || !p.Reserved(uint32(p.Op.CMSGLogoutRequest)) {
		t.Fatal("session opcodes not reserved")
	}
}
