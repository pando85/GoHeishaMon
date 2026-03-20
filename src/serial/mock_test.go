package serial

import (
	"bytes"
	"testing"
)

func makeValidPacket(header byte, length byte, msgType byte) []byte {
	headerBytes := []byte{0x71, length, 0x01, msgType}
	totalLen := int(length) + 3
	dataLen := totalLen - len(headerBytes)
	data := make([]byte, dataLen)

	var checksum byte
	for _, b := range headerBytes {
		checksum += b
	}

	data[dataLen-1] = byte(0x100 - int(checksum))

	packet := make([]byte, 0, totalLen)
	packet = append(packet, headerBytes...)
	packet = append(packet, data...)

	return packet
}

func TestMockReadValidDataPacket(t *testing.T) {
	packet := makeValidPacket(0x71, 0xc8, 0x10)
	mc := newMockComms(packet)

	result := mc.Read()
	if result == nil {
		t.Error("expected valid packet, got nil")
	}
	if len(result) != DataMessageLength {
		t.Errorf("expected length %d, got %d", DataMessageLength, len(result))
	}
}

func TestMockReadValidOptionalPacket(t *testing.T) {
	packet := makeValidPacket(0x71, 0x11, 0x50)
	mc := newMockComms(packet)

	result := mc.Read()
	if result == nil {
		t.Error("expected valid packet, got nil")
	}
	if len(result) != OptionalMessageLength {
		t.Errorf("expected length %d, got %d", OptionalMessageLength, len(result))
	}
}

func TestMockReadInvalidChecksum(t *testing.T) {
	packet := []byte{0x71, 0xc8, 0x01, 0x10}
	packet = append(packet, make([]byte, 199)...)

	mc := newMockComms(packet)

	result := mc.Read()
	if result != nil {
		t.Error("expected nil for invalid checksum, got packet")
	}
}

func TestMockReadPartialPacket(t *testing.T) {
	partialPacket := []byte{0x71, 0xc8, 0x01, 0x10}
	mc := newMockComms(partialPacket)

	result := mc.Read()
	if result != nil {
		t.Error("expected nil for partial packet, got packet")
	}
}

func TestMockReadMultiplePackets(t *testing.T) {
	packet1 := makeValidPacket(0x71, 0xc8, 0x10)
	packet2 := makeValidPacket(0x71, 0x11, 0x50)

	combined := append(packet1, packet2...)
	mc := newMockComms(combined)

	result1 := mc.Read()
	if result1 == nil {
		t.Error("expected first packet")
	}
	if len(result1) != DataMessageLength {
		t.Errorf("expected first packet length %d, got %d", DataMessageLength, len(result1))
	}

	result2 := mc.Read()
	if result2 == nil {
		t.Error("expected second packet")
	}
	if len(result2) != OptionalMessageLength {
		t.Errorf("expected second packet length %d, got %d", OptionalMessageLength, len(result2))
	}
}

func TestMockReadGarbageThenValidPacket(t *testing.T) {
	garbage := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	packet := makeValidPacket(0x71, 0xc8, 0x10)

	combined := append(garbage, packet...)
	mc := newMockComms(combined)

	result := mc.Read()
	if result == nil {
		t.Error("expected valid packet after garbage")
	}
	if len(result) != DataMessageLength {
		t.Errorf("expected packet length %d, got %d", DataMessageLength, len(result))
	}
}

func TestMockWriteCommand(t *testing.T) {
	command := []byte{0xf1, 0x6c, 0x01, 0x10, 0x01}
	mc := newMockComms([]byte{})

	err := mc.SendCommand(command)
	if err != nil {
		t.Errorf("SendCommand failed: %v", err)
	}

	written := mc.mockPort.GetWrittenData()
	if len(written) != len(command)+1 {
		t.Errorf("expected %d bytes written, got %d", len(command)+1, len(written))
	}

	if !bytes.Equal(written[:len(command)], command) {
		t.Error("command bytes don't match")
	}

	expectedChecksum := calcChecksum(command)
	if written[len(command)] != expectedChecksum {
		t.Errorf("checksum mismatch: expected 0x%02x, got 0x%02x", expectedChecksum, written[len(command)])
	}
}

func TestMockStatistics(t *testing.T) {
	packet := makeValidPacket(0x71, 0xc8, 0x10)
	mc := newMockComms(packet)

	stats := mc.GetStatistics()
	if stats.TotalReads != 0 {
		t.Errorf("initial TotalReads should be 0, got %d", stats.TotalReads)
	}

	mc.Read()

	stats = mc.GetStatistics()
	if stats.TotalReads != 1 {
		t.Errorf("TotalReads should be 1, got %d", stats.TotalReads)
	}
	if stats.GoodReads != 1 {
		t.Errorf("GoodReads should be 1, got %d", stats.GoodReads)
	}
}

func TestMockReadWithError(t *testing.T) {
	mc := newMockComms([]byte{})
	mc.mockPort.SetReadError(nil)
	mc.mockPort.readPos = 0
	mc.mockPort.readData = []byte{}

	result := mc.Read()
	if result != nil {
		t.Error("expected nil when no data available")
	}
}
