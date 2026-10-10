// Package auth implements the authserver login protocol for the client builds in
// package protocol: WoW 3.3.5a (build 12340) against AzerothCore and WoW 1.12.1
// (build 5875) against cMaNGOS Classic.
package auth

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/agent-wow/agent-wow/pkg/protocol"
)

// Client authenticates accounts and checks sessions with one authserver.
// Each operation opens and closes its own connection.
type Client struct {
	address string
	profile *protocol.Profile
}

// NewClient creates a client for an authserver at host:port without connecting.
// The profile selects the client build announced to the server.
func NewClient(address string, profile *protocol.Profile) (*Client, error) {
	if profile == nil {
		return nil, errors.New("protocol profile must not be nil")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || strings.TrimSpace(host) == "" {
		return nil, errors.New("authserver address must be host:port")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, errors.New("authserver port must be between 1 and 65535")
	}
	return &Client{address: address, profile: profile}, nil
}
