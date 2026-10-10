// Package classic defines the world packet identifiers of the WoW 1.12.1
// (build 5875) client as cMaNGOS Classic numbers them. Constant names follow
// package opcode; the values differ from build 12340. Authserver and Warden
// payload identifiers are shared with package opcode.
package classic

// WorldName returns the protocol label for a world packet opcode defined in
// this package, or "UNKNOWN" for an unrecognized value.
func WorldName(op uint32) string {
	if op < uint32(len(worldNames)) {
		return worldNames[op]
	}
	return "UNKNOWN"
}
