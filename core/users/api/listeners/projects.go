package listeners

import (
	"context"
	"github.com/devpies/devpie-client-core/users/domain/projects"
	"github.com/devpies/devpie-client-events/go/events"
	"github.com/nats-io/stan.go"
)

func (l *Listeners) handleProjectCreated(m *stan.Msg) {
	msg, err := events.UnmarshalProjectCreatedEvent(m.Data)
	if err != nil {
		l.log.Printf("warning: failed to unmarshal Command \n %v", err)
	}

	event := msg.Data

	updatedtime, err := events.ParseTime(event.UpdatedAt)
	if err != nil {
		l.log.Printf("failed to parse time")
	}
	createdtime, err := events.ParseTime(event.CreatedAt)
	if err != nil {
		l.log.Printf("failed to parse time")
	}

	update := projects.ProjectCopy{
		ID:          event.ProjectID,
		Name:        event.Name,
		UserID:      event.UserID,
		TeamID:      event.TeamID,
		Active:      event.Active,
		Public:      event.Public,
		ColumnOrder: event.ColumnOrder,
		UpdatedAt:   updatedtime,
		CreatedAt:   createdtime,
	}

	if err = projects.Create(context.Background(), l.repo, update); err != nil {
		l.log.Printf("failed to update project: %s \n %v", event.ProjectID, err)
	}

	err = m.Ack()
	if err != nil {
		l.log.Printf("failed to Acknowledge message \n %v", err)
	}
}

func (l *Listeners) handleProjectUpdated(m *stan.Msg) {
	msg, err := events.UnmarshalProjectUpdatedEvent(m.Data)
	if err != nil {
		l.log.Printf("warning: failed to unmarshal Command \n %v", err)
	}

	event := msg.Data

	updatedtime, err := events.ParseTime(event.UpdatedAt)
	if err != nil {
		l.log.Printf("failed to parse time")
	}

	update := projects.UpdateProjectCopy{
		Name:        event.Name,
		Active:      event.Active,
		Public:      event.Public,
		TeamID:      event.TeamID,
		ColumnOrder: event.ColumnOrder,
		UpdatedAt:   updatedtime,
	}

	if err = projects.Update(context.Background(), l.repo, event.ProjectID, update); err != nil {
		l.log.Printf("failed to update project: %s \n %v", event.ProjectID, err)
	}

	err = m.Ack()
	if err != nil {
		l.log.Printf("failed to Acknowledge message \n %v", err)
	}
}

func (l *Listeners) handleProjectDeleted(m *stan.Msg) {
	msg, err := events.UnmarshalProjectDeletedEvent(m.Data)
	if err != nil {
		l.log.Printf("warning: failed to unmarshal Command \n %v", err)
	}

	event := msg.Data

	if err := projects.Delete(context.Background(), l.repo, event.ProjectID); err != nil {
		l.log.Printf("failed to delete project: %s \n %v", event.ProjectID, err)
	}

	err = m.Ack()
	if err != nil {
		l.log.Printf("failed to Acknowledge message \n %v", err)
	}
}
