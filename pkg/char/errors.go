package char

import "fmt"

// ServerError preserves an explicit server rejection's protocol result code.
type ServerError struct {
	Operation string
	Code      uint8
	// messages names the codes of the build that returned Code; nil uses 3.3.5a.
	messages map[uint8]string
}

func (e *ServerError) Error() string {
	messages := e.messages
	if messages == nil {
		messages = wrathCodes.messages
	}
	message := messages[e.Code]
	if message == "" {
		message = "unknown server result"
	}
	return fmt.Sprintf("character %s: %s (0x%02x)", e.Operation, message, e.Code)
}

// OutcomeUnknownError means the request may have reached the server, but its
// result could not be confirmed. Do not retry without checking the live list.
type OutcomeUnknownError struct {
	Operation string
	Name      string
	GUID      GUID
	Err       error
}

func (e *OutcomeUnknownError) Error() string {
	target := fmt.Sprintf("GUID %d", e.GUID)
	if e.Operation == "create" {
		target = fmt.Sprintf("name %q", e.Name)
	}
	return fmt.Sprintf("character %s outcome unknown for %s; run 'agent-wow char list' before retrying: %v", e.Operation, target, e.Err)
}

func (e *OutcomeUnknownError) Unwrap() error { return e.Err }
