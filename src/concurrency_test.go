package main

import (
	"errors"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/pando85/GoHeishaMon/src/serial"
)

// These tests are meant to be run with -race: they exercise state shared
// between goroutines (MQTT handlers, the main send loop, the serial reader and
// the actData refresher). Unsynchronised access to it corrupted memory on the
// device (e.g. a fmt buffer with len 64 > cap 21 panicking in os.File.Write).

type fakeClient struct{ mqtt.Client }

func (fakeClient) Publish(string, byte, bool, interface{}) mqtt.Token {
	return &mqtt.DummyToken{}
}

type fakeMessage struct {
	mqtt.Message
	payload []byte
}

func (m fakeMessage) Payload() []byte { return m.payload }

func TestCommandQueueConcurrentAccess(t *testing.T) {
	const n = 200
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			HandleSetQuietMode(nil, fakeMessage{payload: []byte("1")})
		}
	}()

	received := 0
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	for finished := false; !finished; {
		select {
		case <-done:
			finished = true
		default:
		}
		for {
			command, _ := nextCommand()
			if command == nil {
				break
			}
			received++
		}
	}

	if received != n {
		t.Fatalf("received %d commands, want %d", received, n)
	}
}

func TestNextCommandEmpty(t *testing.T) {
	for command, _ := nextCommand(); command != nil; command, _ = nextCommand() {
	}
	command, pending := nextCommand()
	if command != nil || pending != 0 {
		t.Fatalf("nextCommand() on empty queue = %v, %d; want nil, 0", command, pending)
	}
}

func TestActDataConcurrentRefresh(t *testing.T) {
	ParseTopicList3()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				clearActData()
			}
		}
	}()

	packet := make([]byte, 203)
	for i := 0; i < 50; i++ {
		decode_heatpump_data(packet, fakeClient{})
	}
	close(stop)
	wg.Wait()
}

func TestCommandQueueIsFIFO(t *testing.T) {
	for command, _ := nextCommand(); command != nil; command, _ = nextCommand() {
	}
	// e.g. "quiet mode off" then "powerful on" must reach the heat pump in that order
	for i := byte(1); i <= 5; i++ {
		queueCommand([]byte{i})
	}
	for i := byte(1); i <= 5; i++ {
		command, pending := nextCommand()
		if len(command) != 1 || command[0] != i {
			t.Fatalf("command %d = %v, want [%d]", i, command, i)
		}
		if want := int(5 - i); pending != want {
			t.Errorf("after command %d pending = %d, want %d", i, pending, want)
		}
	}
}

func drainQueueForTest() {
	for command, _ := nextCommand(); command != nil; command, _ = nextCommand() {
	}
}

func TestQueuedCommandSurvivesUnavailablePort(t *testing.T) {
	drainQueueForTest()
	defer drainQueueForTest()
	queueCommand([]byte{1})
	queueCommand([]byte{2})
	attempted, err := sendNextQueuedCommand(func([]byte) error {
		return serial.ErrNotOpen
	})
	if !attempted || !errors.Is(err, serial.ErrNotOpen) {
		t.Fatalf("send attempt = %v, %v; expected unavailable port", attempted, err)
	}
	if command, pending := peekCommand(); len(command) != 1 || command[0] != 1 || pending != 1 {
		t.Fatalf("queue after unavailable port = %v (%d pending)", command, pending)
	}
	attempted, err = sendNextQueuedCommand(func(command []byte) error {
		if command[0] != 1 {
			t.Fatalf("recovery sent command %v instead of first queued command", command)
		}
		return nil
	})
	if !attempted || err != nil {
		t.Fatalf("recovered send failed: %v, %v", attempted, err)
	}
	if command, _ := nextCommand(); len(command) != 1 || command[0] != 2 {
		t.Fatalf("second command was not preserved: %v", command)
	}
}

func TestAmbiguousWriteErrorDoesNotReplay(t *testing.T) {
	drainQueueForTest()
	defer drainQueueForTest()
	queueCommand([]byte{1})
	writeErr := errors.New("I/O failure after unknown number of bytes")
	attempted, err := sendNextQueuedCommand(func([]byte) error { return writeErr })
	if !attempted || !errors.Is(err, writeErr) {
		t.Fatalf("attempt = %v, err = %v", attempted, err)
	}
	if command, _ := nextCommand(); command != nil {
		t.Fatalf("ambiguous failed command was retried: %v", command)
	}
}

func TestQueuedCommandsExpire(t *testing.T) {
	drainQueueForTest()
	defer drainQueueForTest()
	queueCommand([]byte{1})
	commandsMu.Lock()
	commandQueue[0].queuedAt = time.Now().Add(-commandTTL - time.Second)
	commandsMu.Unlock()
	if command, _ := peekCommand(); command != nil {
		t.Fatalf("expired command is still queued: %v", command)
	}
}

func TestCommandQueueIsBounded(t *testing.T) {
	drainQueueForTest()
	defer drainQueueForTest()
	for i := 0; i < maxQueuedCommands; i++ {
		queueCommand([]byte{1})
	}
	queueCommand([]byte{2}) // rejected: the queue is full
	for i := 0; i < maxQueuedCommands; i++ {
		command, _ := nextCommand()
		if len(command) != 1 || command[0] != 1 {
			t.Fatalf("queued element %d = %v, want [1]", i, command)
		}
	}
	if command, _ := nextCommand(); command != nil {
		t.Fatalf("queue exceeded capacity: %v", command)
	}
}

type availabilityMessage struct {
	topic string
	qos byte
	retained bool
	status string
}

type availabilityClient struct {
	mqtt.Client
	messages []availabilityMessage
}

func (c *availabilityClient) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	c.messages = append(c.messages, availabilityMessage{
		topic: topic,
		qos: qos,
		retained: retained,
		status: payload.(string),
	})
	return &mqtt.DummyToken{}
}

func TestSerialAvailabilityReportsOutagesAndRecovery(t *testing.T) {
	config.Mqtt_topic_base = "heatpump"
	client := &availabilityClient{}
	state := &serialAvailability{}
	state.update(client, 0, false) // startup
	state.update(client, 0, true)  // healthy reply
	state.update(client, 0, true)  // retained state refreshed
	state.update(client, 1, false) // link lost, even if already reopened
	state.update(client, 1, false) // no repeated offline
	state.update(client, 1, true)  // communication recovered
	want := []string{"offline", "online", "online", "offline", "online"}
	if len(client.messages) != len(want) {
		t.Fatalf("availability publications = %d, want %d: %+v", len(client.messages), len(want), client.messages)
	}
	for i, msg := range client.messages {
		if msg.status != want[i] || msg.topic != "heatpump/Status" || !msg.retained || msg.qos != 1 {
			t.Errorf("publish[%d] = %+v, want retained QoS 1 %s", i, msg, want[i])
		}
	}
}
