package ircconn

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"
)

func TestJoinDoesNotHoldLockDuringDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var hmu sync.Mutex
	var held []net.Conn
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			hmu.Lock()
			held = append(held, conn)
			hmu.Unlock()
		}
	}()
	defer func() {
		hmu.Lock()
		for _, c := range held {
			c.Close()
		}
		hmu.Unlock()
	}()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := NewManager("ws://"+ln.Addr().String(), 5, func(string) {}, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go m.Join(ctx, "slowchan")
	time.Sleep(100 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		m.Part(ctx, "otherchan")
		m.Join(ctx, "slowchan")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Part/Join blocked while a dial was in progress: mutex held during dial")
	}
}
