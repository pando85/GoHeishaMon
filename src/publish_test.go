package main

import (
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// stuckToken never completes, like a QoS 0 publish token that paho v1.5.0 loses
// when the connection drops (eclipse-paho/paho.mqtt.golang#798).
type stuckToken struct{ mqtt.Token }

func (stuckToken) Wait() bool { select {} }

func (stuckToken) WaitTimeout(d time.Duration) bool {
	time.Sleep(d)
	return false
}

func (stuckToken) Error() error { return nil }

type stuckClient struct {
	mqtt.Client
	publishes int
}

func (c *stuckClient) Publish(string, byte, bool, interface{}) mqtt.Token {
	c.publishes++
	return stuckToken{}
}

// A lost publish token must not stop decode_heatpump_data, or nothing reads the
// heat pump's replies any more until a restart.
func TestDecodeDoesNotBlockOnLostPublishToken(t *testing.T) {
	savedTimeout := publishTimeout
	publishTimeout = time.Millisecond
	defer func() { publishTimeout = savedTimeout }()
	actDataMu.Lock()
	for k := range actData {
		actData[k] = "nil"
	}
	actDataMu.Unlock()

	c := &stuckClient{}
	done := make(chan struct{})
	go func() {
		decode_heatpump_data(make([]byte, 203), c)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("decode_heatpump_data still blocked after %d publishes", c.publishes)
	}
	if c.publishes == 0 {
		t.Fatal("nothing published")
	}
}
