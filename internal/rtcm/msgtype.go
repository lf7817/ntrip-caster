package rtcm

// ExtractMsgType extracts the message type (DF002) from an RTCM3 frame.
// RTCM3 frame structure: 0xD3 | len(10bit) | payload | CRC
// DF002 (Message Type Number) is a 12-bit field at the start of the payload:
//   - Bits 1-8: byte 3 of frame (first payload byte)
//   - Bits 9-12: high 4 bits of byte 4 of frame
//
// Returns 0 if the frame is too short to extract the message type.
func ExtractMsgType(pkt *RTCMPacket) uint16 {
	data := pkt.Data
	// Minimum frame: 3 header + 2 payload bytes needed for msg type + 3 CRC = 8 bytes
	// But we only need header (3) + 2 bytes for msg type = 5 bytes minimum
	if len(data) < 5 {
		return 0
	}

	// DF002 = 12 bits starting at payload byte 0
	// byte 3 (payload[0]) = high 8 bits
	// byte 4 (payload[1]) high nibble = low 4 bits
	msgType := (uint16(data[3]) << 4) | (uint16(data[4]) >> 4)
	return msgType
}