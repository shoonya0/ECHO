package models

import (
	"sync"
	"testing"
)

func TestTrySendFullBufferReturnsFalse(t *testing.T) {
	c := &Client{Send: make(chan WebSocketMessage, 1)}
	if !c.TrySend(WebSocketMessage{Type: "a"}) {
		t.Fatal("first send into empty buffer should succeed")
	}
	if c.TrySend(WebSocketMessage{Type: "b"}) {
		t.Fatal("send into full buffer should return false")
	}
}

func TestCloseSendTwiceDoesNotPanic(t *testing.T) {
	c := &Client{Send: make(chan WebSocketMessage, 1)}
	c.CloseSend()
	c.CloseSend()
	if _, ok := <-c.Send; ok {
		t.Fatal("Send should be closed")
	}
}

func TestTrySendAfterCloseReturnsFalse(t *testing.T) {
	c := &Client{Send: make(chan WebSocketMessage, 1)}
	c.CloseSend()
	if c.TrySend(WebSocketMessage{}) {
		t.Fatal("send after close should return false, not panic")
	}
}

func TestConcurrentSendAndClose(t *testing.T) {
	c := &Client{Send: make(chan WebSocketMessage, 4)}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); c.TrySend(WebSocketMessage{}) }()
		go func() { defer wg.Done(); c.CloseSend() }()
	}
	wg.Wait()
}
