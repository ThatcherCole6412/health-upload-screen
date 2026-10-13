package screening

import (
	"context"
	"encoding/json"
	"testing"
)

type fixedModerator bool

func (m fixedModerator) Flagged(context.Context, Submission) (bool, error) {
	return bool(m), nil
}

type recordingTransformer struct {
	calls int
}

func (t *recordingTransformer) Resize(context.Context, Submission) (json.RawMessage, error) {
	t.calls++
	return json.RawMessage(`{"id":"prepared-image"}`), nil
}

func TestWorkflowDecision(t *testing.T) {
	tests := []struct {
		name           string
		flagged        bool
		wantState      State
		wantTransforms int
	}{
		{name: "safe upload becomes publish ready", wantState: StatePublishReady, wantTransforms: 1},
		{name: "flagged upload is quarantined", flagged: true, wantState: StateQuarantined, wantTransforms: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer := &recordingTransformer{}
			workflow := NewWorkflow(fixedModerator(tt.flagged), transformer)
			submission := Submission{
				ID: "upload-42", Appointment: "appt-17", Filename: "check-in.jpg",
				Caption: "Photo for my scheduled visit", Image: []byte("image bytes"),
			}

			got, err := workflow.Screen(context.Background(), submission)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tt.wantState {
				t.Fatalf("state = %q, want %q", got.State, tt.wantState)
			}
			if transformer.calls != tt.wantTransforms {
				t.Fatalf("resize calls = %d, want %d", transformer.calls, tt.wantTransforms)
			}
		})
	}
}

func TestWorkflowReusesCompletedDecision(t *testing.T) {
	transformer := &recordingTransformer{}
	workflow := NewWorkflow(fixedModerator(false), transformer)
	submission := Submission{
		ID: "upload-42", Appointment: "appt-17", Filename: "check-in.jpg",
		Caption: "Photo for my scheduled visit", Image: []byte("image bytes"),
	}
	for i := 0; i < 2; i++ {
		if _, err := workflow.Screen(context.Background(), submission); err != nil {
			t.Fatal(err)
		}
	}
	if transformer.calls != 1 {
		t.Fatalf("resize calls = %d, want 1", transformer.calls)
	}
}
