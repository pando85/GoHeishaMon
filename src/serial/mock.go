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
	m.writeData = append(m.writeData, p...)
	return len(p), nil
}

func (m *MockPort) Close() error {
	return nil
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
	mc.Comms.serialPort = mc.mockPort
	mc.buffer.Reset()
	return mc
}
