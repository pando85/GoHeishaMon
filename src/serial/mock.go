package serial

import (
	"io"
)

var _ SerialPort = (*MockPort)(nil)

type MockPort struct {
	readData  []byte
	readPos   int
	writeData []byte
	readErr   error
	writeErr  error
	writeLimits []int
	closed    bool
}

func NewMockPort(data []byte) *MockPort {
	return &MockPort{
		readData: data,
	}
}

func (m *MockPort) Read(p []byte) (n int, err error) {
	if m.readErr != nil {
		return 0, m.readErr
	}
	if m.readPos >= len(m.readData) {
		return 0, io.EOF
	}
	n = copy(p, m.readData[m.readPos:])
	m.readPos += n
	return n, nil
}

func (m *MockPort) Write(p []byte) (n int, err error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	n = len(p)
	if len(m.writeLimits) > 0 {
		n = m.writeLimits[0]
		m.writeLimits = m.writeLimits[1:]
		if n < 0 {
			n = 0
		}
		if n > len(p) {
			n = len(p)
		}
	}
	m.writeData = append(m.writeData, p[:n]...)
	return n, nil
}

// SetWriteLimits simulates a series of writes that return fewer bytes and no
// error. Unspecified subsequent writes use the full buffer.
func (m *MockPort) SetWriteLimits(limits ...int) {
	m.writeLimits = append([]int(nil), limits...)
}

func (m *MockPort) Close() error {
	m.closed = true
	return nil
}

func (m *MockPort) IsClosed() bool {
	return m.closed
}

func (m *MockPort) Flush() error {
	return nil
}

func (m *MockPort) GetWrittenData() []byte {
	return m.writeData
}

func (m *MockPort) SetReadError(err error) {
	m.readErr = err
}

func (m *MockPort) SetWriteError(err error) {
	m.writeErr = err
}

type mockComms struct {
	*Comms
	mockPort *MockPort
}

func newMockComms(data []byte) *mockComms {
	mc := &mockComms{
		Comms:    &Comms{},
		mockPort: NewMockPort(data),
	}
	mc.serialPort = mc.mockPort
	mc.buffer.Reset()
	return mc
}
