package screening

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

type State string

const (
	StatePublishReady State = "publish_ready"
	StateQuarantined  State = "quarantined"
)

type Submission struct {
	ID          string
	Appointment string
	Filename    string
	Caption     string
	Image       []byte
}

type Decision struct {
	SubmissionID string          `json:"submission_id"`
	Appointment  string          `json:"appointment_id"`
	State        State           `json:"state"`
	Notification string          `json:"notification"`
	Image        json.RawMessage `json:"image,omitempty"`
}

type Moderator interface {
	Flagged(context.Context, Submission) (bool, error)
}

type Transformer interface {
	Resize(context.Context, Submission) (json.RawMessage, error)
}

type Workflow struct {
	moderator   Moderator
	transformer Transformer
	mu          sync.Mutex
	completed   map[string]Decision
}

func NewWorkflow(m Moderator, t Transformer) *Workflow {
	return &Workflow{moderator: m, transformer: t, completed: make(map[string]Decision)}
}

func (w *Workflow) Screen(ctx context.Context, s Submission) (Decision, error) {
	if s.ID == "" || s.Appointment == "" || s.Filename == "" || s.Caption == "" || len(s.Image) == 0 {
		return Decision{}, errors.New("submission_id, appointment_id, filename, caption, and image are required")
	}

	// Serialize a submission through its decision so a repeated write cannot transform twice.
	w.mu.Lock()
	defer w.mu.Unlock()
	if prior, ok := w.completed[s.ID]; ok {
		return prior, nil
	}

	flagged, err := w.moderator.Flagged(ctx, s)
	if err != nil {
		return Decision{}, fmt.Errorf("moderate submission: %w", err)
	}
	if flagged {
		decision := Decision{
			SubmissionID: s.ID,
			Appointment:  s.Appointment,
			State:        StateQuarantined,
			Notification: "Upload held for staff review before publication.",
		}
		w.completed[s.ID] = decision
		return decision, nil
	}

	image, err := w.transformer.Resize(ctx, s)
	if err != nil {
		return Decision{}, fmt.Errorf("resize approved image: %w", err)
	}
	decision := Decision{
		SubmissionID: s.ID,
		Appointment:  s.Appointment,
		State:        StatePublishReady,
		Notification: "Upload cleared and prepared for the appointment workflow.",
		Image:        image,
	}
	w.completed[s.ID] = decision
	return decision, nil
}
