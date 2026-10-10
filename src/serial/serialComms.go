// Package serial implements serial port communication with the heat pump.package serial
// source: https://github.com/rondoval/GoHeishaMon/blob/b658ee516a86a36ffdcc8bb32ce9edaa8359a4e1/package/heishamon/src/serial/serialComms.go
// credits to @rondoval

package serial

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/pando85/GoHeishaMon/src/logger"
	tarm "github.com/tarm/serial"
)

const dataBufferSize = 1024

type SerialPort interface {
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Close() error
	Flush() error
}

type tarmPort struct {
	*tarm.Port
}

func (t *tarmPort) Read(p []byte) (n int, err error) {
	return t.Port.Read(p)
}

func (t *tarmPort) Write(p []byte) (n int, err error) {
	return t.Port.Write(p)
}

func (t *tarmPort) Close() error {
	return t.Port.Close()
}

func (t *tarmPort) Flush() error {
	return t.Port.Flush()
}

var _ SerialPort = (*tarmPort)(nil)

// OptionalMessageLength is a length of an Optional PCB datagram with checksum
const OptionalMessageLength = 20

// DataMessageLength is a length of an IoT device datagram with a checksum
const DataMessageLength = 203

const loggingRatio = 150

// ErrNotOpen is returned by SendCommand while the port is down and waiting to
// be reopened; the reopen attempts are logged, so callers needn't log it.
var ErrNotOpen = errors.New("serial port is not open")

const (
	reopenMinDelay = time.Second
	reopenMaxDelay = 30 * time.Second
	// defaultStallTimeout is how long the port may stay silent before it is
	// reopened: a vanished USB adapter makes reads return no data and no error.
	defaultStallTimeout = 30 * time.Second
)

// Comms represents a serial port used to communicate with the heat pump.
// Handles low level communications, i.e. packet assembly, checksum generation/verification etc.
type Comms struct {
	goodreads    int64
	totalreads   int64
	buffer       bytes.Buffer
	// portMu guards the port across reads, writes and reconnection, and the
	// fields below it.
	portMu       sync.Mutex
	serialPort   SerialPort // nil while the port is down
	serialConfig *tarm.Config
	nextReopen   time.Time
	reopenDelay  time.Duration
	lastRx       time.Time
	linkLosses   uint64

	// StallTimeout overrides defaultStallTimeout; set it before the first Read.
	StallTimeout time.Duration

	// for tests; nil means the real port, clock and sleep
	openPort func(*tarm.Config) (SerialPort, error)
	now      func() time.Time
	sleep    func(time.Duration)
}

// Statistics contains read statistics
type Statistics struct {
	GoodReads      int64
	TotalReads     int64
	ReadPercentage float64
}

// Open opens the serial port and initializes internal structures.
// If it fails, Read and SendCommand keep retrying with a growing delay.
func (s *Comms) Open(portName string, timeout time.Duration) error {
	s.portMu.Lock()
	defer s.portMu.Unlock()
	s.serialConfig = &tarm.Config{
		Name:        portName,
		Baud:        9600,
		Parity:      tarm.ParityEven,
		StopBits:    tarm.Stop1,
		ReadTimeout: timeout,
	}
	return s.openInternal()
}

func (s *Comms) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Comms) openInternal() error {
	logger.Info("Opening serial port")
	var port SerialPort
	var err error
	if s.openPort != nil {
		port, err = s.openPort(s.serialConfig)
	} else {
		var p *tarm.Port
		if p, err = tarm.OpenPort(s.serialConfig); err == nil {
			port = &tarmPort{Port: p}
		}
	}
	if err != nil {
		if s.reopenDelay < reopenMinDelay {
			s.reopenDelay = reopenMinDelay
		}
		s.nextReopen = s.clock().Add(s.reopenDelay)
		s.reopenDelay *= 2
		if s.reopenDelay > reopenMaxDelay {
			s.reopenDelay = reopenMaxDelay
		}
		return fmt.Errorf("failed to open serial port: %w", err)
	}
	s.serialPort = port
	s.lastRx = s.clock()

	logger.Debug("Flushing serial port buffer")
	if err := s.serialPort.Flush(); err != nil {
		logger.Error("Failed to flush serial port: %v", err)
	}

	return nil
}

// ensureOpen reopens a port that is down, at most once per reopen delay.
// Callers hold portMu.
func (s *Comms) ensureOpen() bool {
	if s.serialPort != nil {
		return true
	}
	if s.serialConfig == nil || s.clock().Before(s.nextReopen) {
		return false
	}
	if err := s.openInternal(); err != nil {
		logger.Error("%v (next attempt in %v)", err, s.nextReopen.Sub(s.clock()))
		return false
	}
	logger.Info("Serial port reopened")
	return true
}

// linkLost closes the port right away, which also frees the device name, so
// a re-plugged USB adapter comes back under the same name. Callers hold portMu.
func (s *Comms) linkLost(reason error) {
	logger.Error("Serial link lost: %v; reopening the port", reason)
	if err := s.closeInternal(); err != nil {
		logger.Error("Failed to close serial port: %v", err)
	}
	// The next connection must not reuse bytes from an incomplete old frame.
	s.buffer.Reset()
	s.linkLosses++
	s.nextReopen = s.clock()
}

// LinkLossCount lets the MQTT reader observe disconnections even if another
// goroutine has already reopened the port before it polls again.
func (s *Comms) LinkLossCount() uint64 {
	s.portMu.Lock()
	defer s.portMu.Unlock()
	return s.linkLosses
}

// Close closes the serial port.
func (s *Comms) Close() error {
	s.portMu.Lock()
	defer s.portMu.Unlock()
	return s.closeInternal()
}

func (s *Comms) closeInternal() error {
	if s.serialPort == nil {
		return nil
	}
	err := s.serialPort.Close()
	s.serialPort = nil
	return err
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

	s.portMu.Lock()
	defer s.portMu.Unlock()
	if !s.ensureOpen() {
		return ErrNotOpen
	}
	n, err := s.serialPort.Write(command) // first send command
	if err == nil && n != len(command) {
		err = io.ErrShortWrite
	}
	if err != nil {
		err = fmt.Errorf("failed to write command: %w", err)
		s.linkLost(err)
		return err
	}
	n, err = s.serialPort.Write([]byte{chk}) // then calculated checksum byte afterwards
	if err == nil && n != 1 {
		err = io.ErrShortWrite
	}
	if err != nil {
		err = fmt.Errorf("failed to write checksum: %w", err)
		s.linkLost(err)
		return err
	}

	logger.DebugHex("Send", command)

	return nil
}

func (s *Comms) readToBuffer() {
	data := make([]byte, dataBufferSize)
	s.portMu.Lock()
	if !s.ensureOpen() {
		s.portMu.Unlock()
		// stands in for the read timeout so the caller's loop doesn't spin
		if s.sleep != nil {
			s.sleep(100 * time.Millisecond)
		} else {
			time.Sleep(100 * time.Millisecond)
		}
		return
	}
	defer s.portMu.Unlock()
	n, err := s.serialPort.Read(data)
	if n > 0 {
		s.buffer.Write(data[:n])
		s.lastRx = s.clock()
		s.reopenDelay = 0 // the link works: the next failure retries quickly again
	}
	if err != nil && err != io.EOF {
		s.linkLost(fmt.Errorf("read error: %w", err))
		return
	}
	stall := s.StallTimeout
	if stall <= 0 {
		stall = defaultStallTimeout
	}
	if n == 0 && s.clock().Sub(s.lastRx) > stall {
		s.linkLost(fmt.Errorf("no data for %v", stall))
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
	if s.buffer.Len() < 4 {
		return 0, false
	}
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
	// Parsing is protected by the same lock used when linkLost clears the
	// incomplete receive buffer. A failed write may close the port concurrently.
	s.portMu.Lock()
	defer s.portMu.Unlock()

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
