// Package rtcm provides RTCM3 framing and packet types.
package rtcm

// RTCMPacket is a read-only wrapper around a complete RTCM3 frame.
// Once created, Data must never be modified; it is shared across all
// broadcast recipients to avoid per-client payload copies.
type RTCMPacket struct {
	Data []byte
}
