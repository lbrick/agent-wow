// Package protocol describes the client builds this client can speak. A Profile
// carries the build identity and the world opcodes the core itself handles;
// packages branch on Profile.Build where a wire format differs between builds.
// Gameplay packets stay opaque to the core, so modules parse them per build.
package protocol

import (
	"fmt"
	"strings"

	"github.com/agent-wow/agent-wow/pkg/opcode"
	"github.com/agent-wow/agent-wow/pkg/opcode/classic"
)

// Supported client builds.
const (
	BuildWrath   = 12340 // WoW 3.3.5a against AzerothCore.
	BuildClassic = 5875  // WoW 1.12.1 against cMaNGOS Classic.
)

// DefaultBuild is the build used when the configuration does not choose one.
const DefaultBuild = BuildWrath

// Opcodes lists the world packets the core sends or consumes itself. Fields
// that a build lacks hold zero and are guarded by the Profile's feature flags.
type Opcodes struct {
	SMSGAuthChallenge, CMSGAuthSession, SMSGAuthResponse            uint16
	CMSGCharEnum, SMSGCharEnum, CMSGCharCreate, SMSGCharCreate      uint16
	CMSGCharDelete, SMSGCharDelete                                  uint16
	CMSGPlayerLogin, SMSGLoginVerifyWorld, SMSGCharacterLoginFailed uint16
	CMSGPing, SMSGPong, SMSGTimeSyncReq, CMSGTimeSyncResp           uint16
	CMSGLogoutRequest, SMSGLogoutResponse, SMSGLogoutComplete       uint16
	SMSGWardenData, CMSGWardenData                                  uint16
}

// Profile is the build-specific view of the protocol. Profiles are immutable
// package-level values; compare them by pointer or by Build.
type Profile struct {
	Build   uint16
	Version [3]byte // Major, minor and bugfix version sent in the logon challenge.
	// AuthProtocol is the protocol byte of the authserver logon challenge.
	AuthProtocol byte
	Release      string // Human-readable client release, such as "3.3.5a".
	Server       string // Server family the profile is written against.
	Op           Opcodes
	// TimeSync reports whether the server drives SMSG_TIME_SYNC_REQ and the
	// session waits for the first one before it is in the world.
	TimeSync bool
	// Warden reports whether the client can answer the server's Warden module.
	Warden bool

	worldName func(uint32) string
	limit     uint32
	// reserved lists client opcodes owned by the core; modules may not send them.
	reserved map[uint32]bool
}

// WorldName returns the protocol label for a world opcode of this build, or
// "UNKNOWN" for a value outside the build's catalog.
func (p *Profile) WorldName(op uint32) string {
	if op >= p.limit {
		return "UNKNOWN"
	}
	return p.worldName(op)
}

// WorldOpcode resolves a protocol label such as "SMSG_UPDATE_OBJECT" for this build.
func (p *Profile) WorldOpcode(name string) (uint16, bool) {
	for op := uint32(0); op < p.limit; op++ {
		if p.worldName(op) == name {
			return uint16(op), true
		}
	}
	return 0, false
}

// Reserved reports whether op is a client opcode the core owns, such as
// authentication, login, ping or logout, which modules must not send.
func (p *Profile) Reserved(op uint32) bool { return p.reserved[op] }

func (p *Profile) String() string { return fmt.Sprintf("%s (build %d)", p.Release, p.Build) }

// ForBuild returns the profile for a supported client build.
func ForBuild(build int) (*Profile, error) {
	for _, p := range profiles {
		if int(p.Build) == build {
			return p, nil
		}
	}
	var supported []string
	for _, p := range profiles {
		supported = append(supported, fmt.Sprintf("%d (%s)", p.Build, p.Release))
	}
	return nil, fmt.Errorf("unsupported client build %d; supported builds: %s", build, strings.Join(supported, ", "))
}

func reservedSet(name func(uint32) string, limit uint32, labels ...string) map[uint32]bool {
	set := make(map[uint32]bool, len(labels))
	for _, label := range labels {
		found := false
		for op := uint32(0); op < limit; op++ {
			if name(op) == label {
				set[op], found = true, true
				break
			}
		}
		if !found {
			panic("protocol: reserved opcode " + label + " is not in the catalog")
		}
	}
	return set
}

// Wrath is WoW 3.3.5a (build 12340) against AzerothCore.
var Wrath = &Profile{
	Build:        BuildWrath,
	Version:      [3]byte{3, 3, 5},
	AuthProtocol: 8,
	Release:      "3.3.5a",
	Server:       "AzerothCore",
	Op: Opcodes{
		SMSGAuthChallenge: opcode.SMSGAuthChallenge, CMSGAuthSession: opcode.CMSGAuthSession, SMSGAuthResponse: opcode.SMSGAuthResponse,
		CMSGCharEnum: opcode.CMSGCharEnum, SMSGCharEnum: opcode.SMSGCharEnum, CMSGCharCreate: opcode.CMSGCharCreate, SMSGCharCreate: opcode.SMSGCharCreate,
		CMSGCharDelete: opcode.CMSGCharDelete, SMSGCharDelete: opcode.SMSGCharDelete,
		CMSGPlayerLogin: opcode.CMSGPlayerLogin, SMSGLoginVerifyWorld: opcode.SMSGLoginVerifyWorld, SMSGCharacterLoginFailed: opcode.SMSGCharacterLoginFailed,
		CMSGPing: opcode.CMSGPing, SMSGPong: opcode.SMSGPong, SMSGTimeSyncReq: opcode.SMSGTimeSyncReq, CMSGTimeSyncResp: opcode.CMSGTimeSyncResp,
		CMSGLogoutRequest: opcode.CMSGLogoutRequest, SMSGLogoutResponse: opcode.SMSGLogoutResponse, SMSGLogoutComplete: opcode.SMSGLogoutComplete,
		SMSGWardenData: opcode.SMSGWardenData, CMSGWardenData: opcode.CMSGWardenData,
	},
	TimeSync:  true,
	Warden:    true,
	worldName: opcode.WorldName,
	limit:     opcode.NumMsgTypes,
	reserved: reservedSet(opcode.WorldName, opcode.NumMsgTypes,
		"CMSG_AUTH_SESSION", "CMSG_AUTH_SRP6_BEGIN", "CMSG_AUTH_SRP6_PROOF", "CMSG_AUTH_SRP6_RECODE", "CMSG_REDIRECTION_AUTH_PROOF",
		"CMSG_PLAYER_LOGIN", "CMSG_CHEAT_PLAYER_LOGIN", "CMSG_CHECK_LOGIN_CRITERIA",
		"CMSG_PING", "CMSG_KEEP_ALIVE", "CMSG_TIME_SYNC_RESP", "CMSG_WARDEN_DATA",
		"CMSG_PLAYER_LOGOUT", "CMSG_LOGOUT_REQUEST", "CMSG_LOGOUT_CANCEL"),
}

// Classic is WoW 1.12.1 (build 5875) against cMaNGOS Classic. It has no time
// sync, and the client does not answer Warden: run it against a realm with
// Warden.Enable = 0.
var Classic = &Profile{
	Build:        BuildClassic,
	Version:      [3]byte{1, 12, 1},
	AuthProtocol: 3,
	Release:      "1.12.1",
	Server:       "cMaNGOS Classic",
	Op: Opcodes{
		SMSGAuthChallenge: classic.SMSGAuthChallenge, CMSGAuthSession: classic.CMSGAuthSession, SMSGAuthResponse: classic.SMSGAuthResponse,
		CMSGCharEnum: classic.CMSGCharEnum, SMSGCharEnum: classic.SMSGCharEnum, CMSGCharCreate: classic.CMSGCharCreate, SMSGCharCreate: classic.SMSGCharCreate,
		CMSGCharDelete: classic.CMSGCharDelete, SMSGCharDelete: classic.SMSGCharDelete,
		CMSGPlayerLogin: classic.CMSGPlayerLogin, SMSGLoginVerifyWorld: classic.SMSGLoginVerifyWorld, SMSGCharacterLoginFailed: classic.SMSGCharacterLoginFailed,
		CMSGPing: classic.CMSGPing, SMSGPong: classic.SMSGPong,
		CMSGLogoutRequest: classic.CMSGLogoutRequest, SMSGLogoutResponse: classic.SMSGLogoutResponse, SMSGLogoutComplete: classic.SMSGLogoutComplete,
		SMSGWardenData: classic.SMSGWardenData, CMSGWardenData: classic.CMSGWardenData,
	},
	worldName: classic.WorldName,
	limit:     classic.NumMsgTypes,
	reserved: reservedSet(classic.WorldName, classic.NumMsgTypes,
		"CMSG_AUTH_SESSION", "CMSG_AUTH_SRP6_BEGIN", "CMSG_AUTH_SRP6_PROOF", "CMSG_AUTH_SRP6_RECODE",
		"CMSG_PLAYER_LOGIN", "CMSG_PING", "CMSG_WARDEN_DATA",
		"CMSG_PLAYER_LOGOUT", "CMSG_LOGOUT_REQUEST", "CMSG_LOGOUT_CANCEL"),
}

var profiles = []*Profile{Wrath, Classic}
