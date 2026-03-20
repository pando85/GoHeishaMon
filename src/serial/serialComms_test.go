package serial

import (
	"testing"
)

func TestCalcChecksum(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected byte
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: 0x00,
		},
		{
			name:     "single byte zero",
			input:    []byte{0x00},
			expected: 0x00,
		},
		{
			name:     "single byte one",
			input:    []byte{0x01},
			expected: 0xFF,
		},
		{
			name:     "data message header",
			input:    []byte{0xf1, 0x6c, 0x01, 0x10},
			expected: 0x92,
		},
		{
			name:     "typical command",
			input:    []byte{0xf1, 0x6c, 0x01, 0x10, 0x01},
			expected: 0x91,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calcChecksum(tt.input)
			if result != tt.expected {
				t.Errorf("calcChecksum() = 0x%02x, expected 0x%02x", result, tt.expected)
			}
		})
	}
}

func TestIsValidReceiveChecksum(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "valid checksum - zeros sum to zero",
			data:     []byte{0x00, 0x00, 0x00, 0x00},
			expected: true,
		},
		{
			name:     "valid checksum - sums to zero",
			data:     []byte{0x01, 0x02, 0xFD, 0x00},
			expected: true,
		},
		{
			name:     "valid checksum - 256 wraps to zero",
			data:     []byte{0xFF, 0x01, 0x00},
			expected: true,
		},
		{
			name:     "invalid checksum",
			data:     []byte{0x01, 0x02, 0x03, 0x00},
			expected: false,
		},
		{
			name:     "empty data",
			data:     []byte{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidReceiveChecksum(tt.data)
			if result != tt.expected {
				t.Errorf("isValidReceiveChecksum() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestChecksumRoundtrip(t *testing.T) {
	command := []byte{0xf1, 0x6c, 0x01, 0x10, 0x01}
	checksum := calcChecksum(command)

	packet := make([]byte, len(command)+1)
	copy(packet, command)
	packet[len(command)] = checksum

	if !isValidReceiveChecksum(packet) {
		t.Errorf("checksum roundtrip failed: packet with checksum should be valid")
	}
}

func TestFindHeaderStart(t *testing.T) {
	tests := []struct {
		name          string
		bufferContent []byte
		expectFound   bool
	}{
		{
			name:          "header at start",
			bufferContent: []byte{0x71, 0xc8, 0x01, 0x10},
			expectFound:   true,
		},
		{
			name:          "header with prefix",
			bufferContent: []byte{0x00, 0x00, 0x71, 0xc8, 0x01, 0x10},
			expectFound:   true,
		},
		{
			name:          "no header",
			bufferContent: []byte{0x00, 0x01, 0x02, 0x03},
			expectFound:   false,
		},
		{
			name:          "empty buffer",
			bufferContent: []byte{},
			expectFound:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Comms{}
			s.buffer.Write(tt.bufferContent)

			result := s.findHeaderStart()
			if result != tt.expectFound {
				t.Errorf("findHeaderStart() = %v, expected %v", result, tt.expectFound)
			}
		})
	}
}

func TestCheckHeader(t *testing.T) {
	tests := []struct {
		name          string
		bufferContent []byte
		expectLength  int
		expectOK      bool
	}{
		{
			name:          "valid data message header",
			bufferContent: []byte{0x71, 0xc8, 0x01, 0x10},
			expectLength:  203,
			expectOK:      true,
		},
		{
			name:          "valid optional message header",
			bufferContent: []byte{0x71, 0x11, 0x01, 0x50},
			expectLength:  20,
			expectOK:      true,
		},
		{
			name:          "invalid header - wrong first byte",
			bufferContent: []byte{0x72, 0xc8, 0x01, 0x10},
			expectLength:  203,
			expectOK:      false,
		},
		{
			name:          "invalid header - wrong third byte",
			bufferContent: []byte{0x71, 0xc8, 0x02, 0x10},
			expectLength:  203,
			expectOK:      false,
		},
		{
			name:          "invalid header - wrong fourth byte",
			bufferContent: []byte{0x71, 0xc8, 0x01, 0x20},
			expectLength:  203,
			expectOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Comms{}
			s.buffer.Write(tt.bufferContent)

			length, ok := s.checkHeader()
			if ok != tt.expectOK {
				t.Errorf("checkHeader() ok = %v, expected %v", ok, tt.expectOK)
			}
			if length != tt.expectLength {
				t.Errorf("checkHeader() length = %d, expected %d", length, tt.expectLength)
			}
		})
	}
}

func TestGetStatistics(t *testing.T) {
	tests := []struct {
		name          string
		goodReads     int64
		totalReads    int64
		expectPercent float64
	}{
		{
			name:          "no reads",
			goodReads:     0,
			totalReads:    0,
			expectPercent: 0,
		},
		{
			name:          "all good reads",
			goodReads:     100,
			totalReads:    100,
			expectPercent: 100.0,
		},
		{
			name:          "half good reads",
			goodReads:     50,
			totalReads:    100,
			expectPercent: 50.0,
		},
		{
			name:          "some errors",
			goodReads:     95,
			totalReads:    100,
			expectPercent: 95.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Comms{
				goodreads:  tt.goodReads,
				totalreads: tt.totalReads,
			}

			stats := s.GetStatistics()
			if stats.GoodReads != tt.goodReads {
				t.Errorf("GoodReads = %d, expected %d", stats.GoodReads, tt.goodReads)
			}
			if stats.TotalReads != tt.totalReads {
				t.Errorf("TotalReads = %d, expected %d", stats.TotalReads, tt.totalReads)
			}
			if stats.ReadPercentage != tt.expectPercent {
				t.Errorf("ReadPercentage = %f, expected %f", stats.ReadPercentage, tt.expectPercent)
			}
		})
	}
}

func TestDispatchDatagram(t *testing.T) {
	tests := []struct {
		name          string
		bufferContent []byte
		length        int
		expectNil     bool
		expectDataLen int
	}{
		{
			name:          "data message",
			bufferContent: make([]byte, 203),
			length:        DataMessageLength,
			expectNil:     false,
			expectDataLen: 203,
		},
		{
			name:          "optional message",
			bufferContent: make([]byte, 20),
			length:        OptionalMessageLength,
			expectNil:     false,
			expectDataLen: 20,
		},
		{
			name:          "unknown length",
			bufferContent: make([]byte, 50),
			length:        50,
			expectNil:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Comms{}
			s.buffer.Write(tt.bufferContent)

			result := s.dispatchDatagram(tt.length)
			if tt.expectNil && result != nil {
				t.Errorf("expected nil result, got %v", result)
			}
			if !tt.expectNil {
				if result == nil {
					t.Errorf("expected non-nil result")
				} else if len(result) != tt.expectDataLen {
					t.Errorf("result length = %d, expected %d", len(result), tt.expectDataLen)
				}
			}
		})
	}
}

func TestBufferManagement(t *testing.T) {
	s := &Comms{}

	s.buffer.Write([]byte{0x00, 0x00, 0x71})
	found := s.findHeaderStart()
	if !found {
		t.Error("expected to find header 0x71")
	}

	remaining := s.buffer.Bytes()
	if len(remaining) != 1 || remaining[0] != 0x71 {
		t.Errorf("buffer should contain only 0x71 after findHeaderStart, got %v", remaining)
	}
}

func TestPacketAssembly(t *testing.T) {
	header := []byte{0x71, 0xc8, 0x01, 0x10}
	data := make([]byte, 199)

	var checksum byte
	for _, b := range header {
		checksum += b
	}
	for _, b := range data {
		checksum += b
	}
	data[len(data)-1] = byte((256 - int(checksum)) % 256)

	packet := make([]byte, 0, 203)
	packet = append(packet, header...)
	packet = append(packet, data...)

	if len(packet) != 203 {
		t.Errorf("packet length = %d, expected 203", len(packet))
	}

	if !isValidReceiveChecksum(packet) {
		t.Error("packet checksum should be valid")
	}

	s := &Comms{}
	s.buffer.Write(packet)

	if !s.findHeaderStart() {
		t.Error("should find header")
	}

	length, ok := s.checkHeader()
	if !ok {
		t.Error("header should be valid")
	}
	if length != 203 {
		t.Errorf("expected length 203, got %d", length)
	}
}

func BenchmarkCalcChecksum(b *testing.B) {
	command := []byte{0xf1, 0x6c, 0x01, 0x10, 0x00, 0x00, 0x00, 0x00}
	for i := 0; i < b.N; i++ {
		calcChecksum(command)
	}
}

func BenchmarkIsValidReceiveChecksum(b *testing.B) {
	data := make([]byte, 203)
	for i := 0; i < b.N; i++ {
		isValidReceiveChecksum(data)
	}
}
