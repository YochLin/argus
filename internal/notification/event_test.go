package notification

import (
	"context"
	"errors"
	"testing"
)

type recordingNotifier struct {
	got []Event
	err error
}

func (n *recordingNotifier) Send(ctx context.Context, e Event) error {
	n.got = append(n.got, e)
	return n.err
}

func TestDispatcherPublishFansOutAndSurvivesOneFailure(t *testing.T) {
	failing := &recordingNotifier{err: errors.New("boom")}
	ok := &recordingNotifier{}
	d := NewDispatcher(failing, ok)

	d.Publish(context.Background(), Event{Type: "stop_loss", Text: "AAPL hit stop"})

	if len(failing.got) != 1 || len(ok.got) != 1 {
		t.Fatalf("want both notifiers to receive the event even though one errors, got failing=%d ok=%d", len(failing.got), len(ok.got))
	}
	if ok.got[0].Text != "AAPL hit stop" {
		t.Errorf("Text = %q, want %q", ok.got[0].Text, "AAPL hit stop")
	}
	if ok.got[0].Time.IsZero() {
		t.Error("Publish should default Time when unset")
	}
}
