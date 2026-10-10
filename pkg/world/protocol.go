package world

import (
	"encoding/binary"
	"errors"
	"time"
)

// The core handles connection lifecycle packets only. Gameplay payloads remain
// opaque and are routed unchanged to registered modules by the owner loop.
func (e *engine) handle(op uint16, body []byte) (bool, error) {
	ops, profile := e.op(), e.s.conn.Profile()
	switch {
	case op == ops.SMSGLoginVerifyWorld:
		if len(body) != 20 {
			return false, errors.New("invalid login verification length")
		}
		e.verified = true
		e.checkReady()
	case profile.TimeSync && op == ops.SMSGTimeSyncReq:
		if len(body) != 4 {
			return false, errors.New("invalid time-sync request")
		}
		response := binary.LittleEndian.AppendUint32(nil, binary.LittleEndian.Uint32(body))
		response = binary.LittleEndian.AppendUint32(response, e.timestamp())
		if err := e.write(uint32(ops.CMSGTimeSyncResp), response); err != nil {
			return false, err
		}
		e.synced = true
		e.checkReady()
	case op == ops.SMSGPong:
		if len(body) != 4 {
			return false, errors.New("invalid pong")
		}
		sequence := binary.LittleEndian.Uint32(body)
		if sent, ok := e.pings[sequence]; ok {
			if time.Since(sent) >= e.timing.pong {
				delete(e.pings, sequence)
				return false, nil
			}
			e.latency = uint32(time.Since(sent) / time.Millisecond)
			for id, at := range e.pings {
				if !at.After(sent) {
					delete(e.pings, id)
				}
			}
			e.pongTimer.Reset(e.timing.pong)
		}
	case op == ops.SMSGCharacterLoginFailed:
		if len(body) != 1 {
			return false, errors.New("invalid login rejection")
		}
		return false, &LoginError{Code: body[0]}
	case op == ops.SMSGLogoutResponse:
		if len(body) != 5 || body[4] > 1 {
			return false, errors.New("invalid logout response")
		}
		reason := binary.LittleEndian.Uint32(body)
		if reason != 0 && e.logout != nil {
			op := e.logout
			e.logout = nil
			e.logoutTimer.Stop()
			e.logoutDeadline = nil
			e.s.modules.Resume()
			e.publish()
			op.err = &LogoutError{Reason: reason}
			close(op.done)
		}
	case op == ops.SMSGLogoutComplete:
		if len(body) != 0 {
			return false, errors.New("invalid logout completion")
		}
		return true, nil
	case op == ops.SMSGWardenData:
		reply, err := e.s.conn.WardenResponse(body)
		if err != nil {
			return false, err
		}
		if reply != nil {
			return false, e.write(uint32(ops.CMSGWardenData), reply)
		}
	}
	return false, nil
}
