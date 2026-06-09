package ircconn

import (
	"context"
	"io"
	"testing"

	"github.com/coder/websocket"
)

type fakeSocket struct {
	lines []string
}

func (f *fakeSocket) Write(_ context.Context, _ websocket.MessageType, data []byte) error {
	f.lines = append(f.lines, string(data))
	return nil
}

func (f *fakeSocket) Read(context.Context) (websocket.MessageType, []byte, error) {
	return websocket.MessageText, nil, io.EOF
}

func (f *fakeSocket) CloseNow() error { return nil }

func TestAuthSenderWritesHandshakeJoinAndPrivmsg(t *testing.T) {
	socket := &fakeSocket{}
	sender, err := newAuthSender(context.Background(), socket, "Viewer", "token")
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(context.Background(), "Streamer", " hello\r\nchat "); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"PASS oauth:token\r\n",
		"NICK viewer\r\n",
		"CAP REQ :twitch.tv/tags twitch.tv/commands\r\n",
		"JOIN #streamer\r\n",
		"PRIVMSG #streamer :hello  chat\r\n",
	}
	if len(socket.lines) != len(want) {
		t.Fatalf("line count got %d want %d: %+v", len(socket.lines), len(want), socket.lines)
	}
	for i := range want {
		if socket.lines[i] != want[i] {
			t.Fatalf("line %d got %q want %q", i, socket.lines[i], want[i])
		}
	}
}
