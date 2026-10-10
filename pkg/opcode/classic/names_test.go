package classic

import "testing"

func TestWorldName(t *testing.T) {
	for op, want := range map[uint32]string{
		MSGNullAction: "MSG_NULL_ACTION", CMSGAuthSession: "CMSG_AUTH_SESSION", SMSGCharEnum: "SMSG_CHAR_ENUM",
		SMSGDefenseMessage: "SMSG_DEFENSE_MESSAGE", NumMsgTypes: "UNKNOWN",
	} {
		if got := WorldName(op); got != want {
			t.Errorf("WorldName(%#x) = %q, want %q", op, got, want)
		}
	}
	if CMSGAuthSession != 0x1ed || SMSGAuthChallenge != 0x1ec || SMSGLoginVerifyWorld != 0x236 {
		t.Error("catalog values differ from mangos-classic Opcodes.h")
	}
}
