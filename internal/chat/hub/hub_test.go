package hub

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
)

type fakeJoiner struct {
	joined []string
	parted []string
}

func (f *fakeJoiner) Join(_ context.Context, channel string) {
	f.joined = append(f.joined, channel)
}

func (f *fakeJoiner) Part(_ context.Context, channel string) {
	f.parted = append(f.parted, channel)
}

type fakeSubscriber struct{}

func (fakeSubscriber) Subscribe(_ context.Context, _ string, _ func([]byte)) (func(), error) {
	return func() {}, nil
}

type fakeSender struct {
	session string
	channel string
	text    string
}

func (f *fakeSender) Send(_ context.Context, sessionID, channel, text string) error {
	f.session = sessionID
	f.channel = channel
	f.text = text
	return nil
}

func TestSubscribeRejectsInvalidChannel(t *testing.T) {
	j := &fakeJoiner{}
	h := New(j, fakeSubscriber{}, 4, 0, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c := &client{send: make(chan []byte, 4), channels: make(map[string]struct{})}

	h.subscribe(context.Background(), c, "bad channel")

	if len(j.joined) != 0 {
		t.Fatalf("invalid channel should not join upstream")
	}
	select {
	case msg := <-c.send:
		var frame map[string]string
		if err := json.Unmarshal(msg, &frame); err != nil {
			t.Fatal(err)
		}
		if frame["type"] != "error" || frame["message"] != "invalid channel" {
			t.Fatalf("unexpected error frame: %+v", frame)
		}
	default:
		t.Fatal("expected error frame")
	}
}

func TestSubscribeEmitsStatus(t *testing.T) {
	j := &fakeJoiner{}
	h := New(j, fakeSubscriber{}, 4, 0, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c := &client{send: make(chan []byte, 4), channels: make(map[string]struct{})}

	h.subscribe(context.Background(), c, "ninja")

	if len(j.joined) != 1 || j.joined[0] != "ninja" {
		t.Fatalf("expected upstream join, got %+v", j.joined)
	}
	select {
	case msg := <-c.send:
		var frame map[string]string
		if err := json.Unmarshal(msg, &frame); err != nil {
			t.Fatal(err)
		}
		if frame["type"] != "status" || frame["state"] != "subscribed" {
			t.Fatalf("unexpected status frame: %+v", frame)
		}
	default:
		t.Fatal("expected status frame")
	}
}

func TestSendMessageRejectsUnauthenticatedClient(t *testing.T) {
	h := New(&fakeJoiner{}, fakeSubscriber{}, 4, 0, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c := &client{send: make(chan []byte, 4), channels: make(map[string]struct{})}

	h.sendMessage(context.Background(), c, controlFrame{
		Channel:     "ninja",
		Text:        "hello",
		ClientMsgID: "m1",
	})

	frame := readFrame(t, c)
	if frame["type"] != "message_error" || frame["message"] != "auth_required" || frame["client_msg_id"] != "m1" {
		t.Fatalf("unexpected frame: %+v", frame)
	}
}

func TestSendMessageRejectsInvalidText(t *testing.T) {
	sender := &fakeSender{}
	h := New(&fakeJoiner{}, fakeSubscriber{}, 4, 0, slog.New(slog.NewTextHandler(io.Discard, nil))).WithAuth(nil, sender)
	c := &client{send: make(chan []byte, 4), channels: make(map[string]struct{}), session: "sid"}

	h.sendMessage(context.Background(), c, controlFrame{
		Channel:     "ninja",
		Text:        "",
		ClientMsgID: "m2",
	})

	frame := readFrame(t, c)
	if frame["type"] != "message_error" || frame["message"] != "message is empty" {
		t.Fatalf("unexpected frame: %+v", frame)
	}
	if sender.text != "" {
		t.Fatalf("sender should not be called, got %q", sender.text)
	}
}

func TestSendMessageDispatchesToSender(t *testing.T) {
	sender := &fakeSender{}
	h := New(&fakeJoiner{}, fakeSubscriber{}, 8, 0, slog.New(slog.NewTextHandler(io.Discard, nil))).WithAuth(nil, sender)
	c := &client{send: make(chan []byte, 8), channels: make(map[string]struct{}), session: "sid"}

	h.sendMessage(context.Background(), c, controlFrame{
		Channel:     "Ninja",
		Text:        "hello chat",
		ClientMsgID: "m3",
	})

	queued := readFrame(t, c)
	sent := readFrame(t, c)
	if queued["type"] != "message_ack" || queued["state"] != "queued" {
		t.Fatalf("unexpected queued frame: %+v", queued)
	}
	if sent["type"] != "message_ack" || sent["state"] != "sent" {
		t.Fatalf("unexpected sent frame: %+v", sent)
	}
	if sender.session != "sid" || sender.channel != "ninja" || sender.text != "hello chat" {
		t.Fatalf("unexpected sender call: %+v", sender)
	}
}

func readFrame(t *testing.T, c *client) map[string]any {
	t.Helper()
	select {
	case msg := <-c.send:
		var frame map[string]any
		if err := json.Unmarshal(msg, &frame); err != nil {
			t.Fatal(err)
		}
		return frame
	default:
		t.Fatal("expected frame")
		return nil
	}
}
