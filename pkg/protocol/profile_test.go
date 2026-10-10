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
