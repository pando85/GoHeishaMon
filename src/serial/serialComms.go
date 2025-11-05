// Package serial implements serial port communication with the heat pump.package serial
// source: https://github.com/rondoval/GoHeishaMon/blob/b658ee516a86a36ffdcc8bb32ce9edaa8359a4e1/package/heishamon/src/serial/serialComms.go
// credits to @rondoval

package serial

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/pando85/GoHeishaMon/src/logger"
	tarm "github.com/tarm/serial"
)

const dataBufferSize = 1024

// OptionalMessageLength is a length of an Optional PCB datagram with checksum
const OptionalMessageLength = 20

// DataMessageLength is a length of an IoT device datagram with a checksum
const DataMessageLength = 203

const loggingRatio = 150

// Comms represents a serial port used to communicate with the heat pump.
// Handles low level communications, i.e. packet assembly, checksum generation/verification etc.
type Comms struct {
	goodreads    int64
	totalreads   int64
	buffer       bytes.Buffer
	serialPort   *tarm.Port
	serialConfig *tarm.Config
}

// Statistics contains read statistics
type Statistics struct {
	GoodReads      int64
	TotalReads     int64
	ReadPercentage float64
}

// Open opens the serial port and initializes internal structures.
func (s *Comms) Open(portName string, timeout time.Duration) error {
	s.serialConfig = &tarm.Config{
		Name:        portName,
		Baud:        9600,
		Parity:      tarm.ParityEven,
		StopBits:    tarm.Stop1,
		ReadTimeout: timeout,
	}
	return s.openInternal()
}

func (s *Comms) openInternal() error {
	var err error
	logger.Info("Opening serial port")
	s.serialPort, err = tarm.OpenPort(s.serialConfig)
	if err != nil {
		return fmt.Errorf("failed to open serial port: %w", err)
	}

	logger.Debug("Flushing serial port buffer")
	s.serialPort.Flush()

	return nil
}

// Close closes the serial port.
func (s *Comms) Close() error {
	if s.serialPort != nil {
		return s.serialPort.Close()
	}
	return nil
}

// GetStatistics returns current read statistics
func (s *Comms) GetStatistics() Statistics {
	stats := Statistics{
		GoodReads:  s.goodreads,
		TotalReads: s.totalreads,
	}
	if s.totalreads > 0 {
		stats.ReadPercentage = float64(s.goodreads) / float64(s.totalreads) * 100.0
	}
	return stats
}

func isValidReceiveChecksum(data []byte) bool {
	var chk byte
	for _, v := range data {
		chk += v
	}
	return (chk == 0) // all received bytes + checksum should result in 0
}

func calcChecksum(command []byte) byte {
	var chk byte
	for _, v := range command {
		chk += v
	}
	return (chk ^ 0xFF) + 01
}

// SendCommand sends a datagram to the heat pump.
// Appends checksum.
func (s *Comms) SendCommand(command []byte) error {
	var chk = calcChecksum(command)

	_, err := s.serialPort.Write(command) // first send command
	if err != nil {
		return fmt.Errorf("failed to write command: %w", err)
	}
	_, err = s.serialPort.Write([]byte{chk}) // then calculated checksum byte afterwards
	if err != nil {
		return fmt.Errorf("failed to write checksum: %w", err)
	}

	logger.DebugHex("Send", command)

	return nil
}

func (s *Comms) readToBuffer() {
	data := make([]byte, dataBufferSize)
	n, err := s.serialPort.Read(data)
	if err != nil && err != io.EOF {
		logger.Error("Serial read error: %v", err)
		s.Close()
		// Attempt to reconnect
		if reopenErr := s.openInternal(); reopenErr != nil {
			logger.Error("Failed to reconnect: %v", reopenErr)
		}
	}
	if n > 0 {
		s.buffer.Write(data[:n])
	}
}

func (s *Comms) findHeaderStart() bool {
	if s.buffer.Len() < 1 {
		return false
	}
	hdr := bytes.IndexByte(s.buffer.Bytes(), 0x71)
	if hdr < 0 {
		logger.Debug("No header found, clearing buffer of size %d", s.buffer.Len())
		return false
	} else if hdr > 0 {
		// Found header but not at start, discard bytes before it
		waste := s.buffer.Next(hdr)
		logger.DebugHex(fmt.Sprintf("Discarding %d bytes before header", len(waste)), waste)
	}
	return true
}

func (s *Comms) dispatchDatagram(length int) []byte {
	s.goodreads++
	readpercentage := float64(s.totalreads-s.goodreads) / float64(s.totalreads) * 100.
	if s.totalreads%loggingRatio == 0 {
		logger.Info("RX: %d RX errors: %d (%.2f %%)", s.totalreads, s.totalreads-s.goodreads, readpercentage)
	}

	packet := s.buffer.Next(length)

	logger.DebugHex("Received", packet)

	if length == DataMessageLength || length == OptionalMessageLength {
		return packet
	}

	logger.Info("Received an unknown datagram. Can't decode this (yet?). Length: %d", length)
	return nil
}

func (s *Comms) checkHeader() (length int, ok bool) {
	// opt header: 71 11 01 50; 20 bytes
	// header:     71 c8 01 10; 203 bytes
	data := s.buffer.Bytes()
	length = int(data[1]) + 3
	ok = false
	if data[0] == 0x71 && data[2] == 0x1 && (data[3] == 0x50 || data[3] == 0x10) {
		ok = true
		return
	}
	logger.DebugHex("Invalid header bytes", data[:4])
	return
}

// Read attempts to read heat pump reply. Returns nil if full packet with correct checksum was not assembled.
// It holds state and should be called periodically.
func (s *Comms) Read() []byte {
	s.readToBuffer()

	if s.findHeaderStart() && s.buffer.Len() >= 4 { // have entire header at start of buffer
		var (
			length int
			ok     bool
		)

		if length, ok = s.checkHeader(); !ok {
			// consume byte, it's not a valid header
			_, err := s.buffer.ReadByte()
			if err != nil {
				logger.Error("Buffer read error: %v", err)
			}
			return nil
		}

		if s.buffer.Len() >= length { // have entire packet
			s.totalreads++

			if isValidReceiveChecksum(s.buffer.Bytes()[:length]) {
				return s.dispatchDatagram(length)
			}
			// invalid checksum, need to consume 0x71 and look for another one
			_, err := s.buffer.ReadByte()
			if err != nil {
				logger.Error("Buffer read error: %v", err)
			}

			logger.Error("Invalid checksum on receive!")
		} else {
			logger.Debug("Waiting for more data. Have %d, need %d", s.buffer.Len(), length)
		}
	}
	return nil
}
