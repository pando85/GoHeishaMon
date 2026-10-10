package serial

import (
	"errors"
	"testing"
	"time"

	tarm "github.com/tarm/serial"
)

// fakeLink hands out mock ports from Comms.openPort and controls the clock.
type fakeLink struct {
	t     time.Time
	ports []*MockPort
	opens int
	fail  bool
}

func (f *fakeLink) now() time.Time { return f.t }

func (f *fakeLink) advance(d time.Duration) { f.t = f.t.Add(d) }

func (f *fakeLink) open(*tarm.Config) (SerialPort, error) {
	f.opens++
	if f.fail {
		return nil, errors.New("no such device")
	}
	p := NewMockPort(nil)
	f.ports = append(f.ports, p)
	return p, nil
}

func (f *fakeLink) last() *MockPort { return f.ports[len(f.ports)-1] }

func newFakeComms(t *testing.T, fail bool) (*Comms, *fakeLink) {
	t.Helper()
	f := &fakeLink{t: time.Unix(1000, 0), fail: fail}
	s := &Comms{openPort: f.open, now: f.now, sleep: func(time.Duration) {}}
	err := s.Open("/dev/ttyUSB0", 100*time.Millisecond)
	if (err != nil) != fail {
		t.Fatalf("Open() error = %v, want failure %v", err, fail)
	}
	return s, f
}

func TestNoPortDoesNotPanic(t *testing.T) {
	s := &Comms{} // Open never called
	if err := s.SendCommand([]byte{0x71}); !errors.Is(err, ErrNotOpen) {
		t.Errorf("SendCommand without a port = %v, want ErrNotOpen", err)
	}
	if p := s.Read(); p != nil {
		t.Errorf("Read without a port = %v, want nil", p)
	}
}

func TestFailedOpenRetriesWithBackoff(t *testing.T) {
	s, f := newFakeComms(t, true)
	if f.opens != 1 {
		t.Fatalf("opens after Open = %d, want 1", f.opens)
	}
	if err := s.SendCommand([]byte{0x71}); err == nil {
		t.Error("SendCommand with the port down: expected an error")
	}
	if f.opens != 1 {
		t.Errorf("reopened before the backoff: opens = %d", f.opens)
	}

	// retries after 1 s, then 2 s, 4 s, ... up to 30 s between attempts
	wantGaps := []time.Duration{1, 2, 4, 8, 16, 30, 30}
	for i, gap := range wantGaps {
		f.advance(gap*time.Second - time.Millisecond)
		s.Read()
		if f.opens != i+1 {
			t.Fatalf("attempt %d came before %v: opens = %d", i+2, gap*time.Second, f.opens)
		}
		f.advance(time.Millisecond)
		s.Read()
		if f.opens != i+2 {
			t.Fatalf("attempt %d didn't come after %v: opens = %d", i+2, gap*time.Second, f.opens)
		}
	}

	f.fail = false
	f.advance(30 * time.Second)
	if err := s.SendCommand([]byte{0x71}); err != nil {
		t.Fatalf("SendCommand after the port came back: %v", err)
	}
	if got := f.last().GetWrittenData(); len(got) != 2 {
		t.Errorf("written %v, want command + checksum", got)
	}
}

func TestWriteErrorClosesAndReopens(t *testing.T) {
	s, f := newFakeComms(t, false)
	first := f.last()
	first.SetWriteError(errors.New("input/output error"))

	if err := s.SendCommand([]byte{0x71}); err == nil {
		t.Fatal("expected the write error")
	}
	// closing at once frees the device name, so a re-plugged adapter gets it again
	if !first.IsClosed() {
		t.Error("port not closed after a write error")
	}

	s.Read() // first reopen attempt is immediate
	if f.opens != 2 {
		t.Fatalf("opens = %d, want 2", f.opens)
	}
	if err := s.SendCommand([]byte{0x71}); err != nil {
		t.Errorf("SendCommand on the reopened port: %v", err)
	}
}

func TestReadErrorClosesAndReopens(t *testing.T) {
	s, f := newFakeComms(t, false)
	first := f.last()
	first.SetReadError(errors.New("input/output error"))

	s.Read()
	if !first.IsClosed() {
		t.Error("port not closed after a read error")
	}
	s.Read()
	if f.opens != 2 {
		t.Errorf("opens = %d, want 2", f.opens)
	}
}

// A vanished USB adapter makes reads return no data and no error (seen on a
// CZ-TAW1 with a pl2303), so silence is the only sign on the read side.
func TestStallReconnects(t *testing.T) {
	s, f := newFakeComms(t, false)
	first := f.last()

	f.advance(defaultStallTimeout - time.Second)
	s.Read() // io.EOF, still within the timeout
	if first.IsClosed() {
		t.Fatal("closed before the stall timeout")
	}

	f.advance(2 * time.Second)
	s.Read()
	if !first.IsClosed() {
		t.Fatal("port not closed after the stall timeout")
	}
	s.Read()
	if f.opens != 2 {
		t.Errorf("opens = %d, want 2", f.opens)
	}
}

func TestReceivedDataResetsStallTimer(t *testing.T) {
	s, f := newFakeComms(t, false)
	s.StallTimeout = 10 * time.Second
	for i := 0; i < 5; i++ {
		f.advance(5 * time.Second)
		f.last().readData = append(f.last().readData, 0x00)
		s.Read()
	}
	if f.last().IsClosed() || f.opens != 1 {
		t.Errorf("reconnected while data was arriving: opens = %d", f.opens)
	}
}

// A port that opens but stays silent (heat pump off, cable unplugged at the
// heat pump) must not be reopened at full rate forever.
func TestSilentPortBacksOff(t *testing.T) {
	s, f := newFakeComms(t, false)
	s.StallTimeout = 10 * time.Second
	f.fail = true // reopening fails from now on
	f.advance(11 * time.Second)
	s.Read() // stall: closes the port
	s.Read() // immediate reopen attempt fails, next one in 1 s
	if f.opens != 2 {
		t.Fatalf("opens = %d, want 2", f.opens)
	}
	f.fail = false
	f.advance(time.Second)
	s.Read() // reopens, but the delay stays doubled until data arrives
	f.advance(11 * time.Second)
	s.Read() // stall again
	f.fail = true
	s.Read() // immediate attempt fails
	f.advance(time.Second + 500*time.Millisecond)
	s.Read()
	if f.opens != 4 {
		t.Errorf("retried after 1.5 s although no data came since the last failure: opens = %d", f.opens)
	}
}
