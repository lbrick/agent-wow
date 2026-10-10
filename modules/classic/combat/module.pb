
ä
google/protobuf/any.protogoogle.protobuf"6
Any
type_url (	RtypeUrl
value (RvalueBv
com.google.protobufBAnyProtoPZ,google.golang.org/protobuf/types/known/anypb¢GPBªGoogle.Protobuf.WellKnownTypesbproto3
¾
google/protobuf/empty.protogoogle.protobuf"
EmptyB}
com.google.protobufB
EmptyProtoPZ.google.golang.org/protobuf/types/known/emptypbø¢GPBªGoogle.Protobuf.WellKnownTypesbproto3
§
api/module/v1/session.protoagentwow.module.v1google/protobuf/any.protogoogle/protobuf/empty.proto"?
WorldPacket
opcode (Ropcode
payload (Rpayload"E
SendPacketRequest
opcode (Ropcode
payload (Rpayload"5
ClockResponse$
client_time_ms (RclientTimeMs"u
InvokeModuleRequest
module (	Rmodule
method (	Rmethod.
request (2.google.protobuf.AnyRrequest"D
InvokeModuleResponse,
result (2.google.protobuf.AnyRresult2€
SessionK

SendPacket%.agentwow.module.v1.SendPacketRequest.google.protobuf.EmptyE
GetClock.google.protobuf.Empty!.agentwow.module.v1.ClockResponsea
InvokeModule'.agentwow.module.v1.InvokeModuleRequest(.agentwow.module.v1.InvokeModuleResponseB5Z3github.com/agent-wow/agent-wow/pkg/modules/v1;modv1bproto3
Ø
#modules/classic/combat/module.protoagentwow.classic.combat.v1api/module/v1/session.protogoogle/protobuf/empty.proto"#
AttackRequest
guid (Rguid"@
CastRequest
spell_id (RspellId
target (Rtarget"”
Event
time_ms (RtimeMs
kind (	Rkind
source (Rsource
target (Rtarget
amount (Ramount
detail (	Rdetail"œ
StatusResponse
	attacking (R	attacking
kills (Rkills
	xp_gained (RxpGained9
events (2!.agentwow.classic.combat.v1.EventRevents2ï
CombatK
Attack).agentwow.classic.combat.v1.AttackRequest.google.protobuf.Empty<

StopAttack.google.protobuf.Empty.google.protobuf.EmptyG
Cast'.agentwow.classic.combat.v1.CastRequest.google.protobuf.EmptyL
Status.google.protobuf.Empty*.agentwow.classic.combat.v1.StatusResponseC
OnPacket.agentwow.module.v1.WorldPacket.google.protobuf.EmptyBDZBgithub.com/agent-wow/agent-wow/modules/classic/combat/api;combatv1bproto3