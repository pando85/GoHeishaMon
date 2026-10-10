package main

import (
	"sync"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
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
