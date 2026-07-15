// Package net validates that stream traffic originates from inside the
// private Meshwire network before entrypoint accepts it. Keeping the range
// check in one place means every ingress path enforces the same policy.
package net

import (
	"fmt"
	"net"
	"time"
)

// meshwireCIDR is the private address range Meshwire nodes live in. Anything
// outside it is treated as untrusted and rejected.
const meshwireCIDR = "10.64.0.0/16"

// StreamPacket is one event carried over a Meshwire connection.
type StreamPacket struct {
	EventID   string
	Type      string
	Timestamp time.Time
	Actor     string
	Payload   string
}

// VerifyMeshwireInterface reports whether serverAddr falls inside the private
// Meshwire range (10.64.0.0/16). It returns true for an in-range address, and
// an error for a malformed address or one outside the allowed scope.
func VerifyMeshwireInterface(serverAddr string) (bool, error) {
	ip := net.ParseIP(serverAddr)
	if ip == nil {
		return false, fmt.Errorf("net: %q is not a valid IP address", serverAddr)
	}

	_, meshNet, err := net.ParseCIDR(meshwireCIDR)
	if err != nil {
		// meshwireCIDR is a compile-time constant, so this is unreachable in
		// practice; surfaced rather than panicked to keep callers in control.
		return false, fmt.Errorf("net: invalid mesh CIDR %q: %w", meshwireCIDR, err)
	}

	if !meshNet.Contains(ip) {
		return false, fmt.Errorf(
			"net: %s is out of the allowed internal network scope %s", serverAddr, meshwireCIDR)
	}
	return true, nil
}
