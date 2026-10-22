package domain

import (
	"context"

	"github.com/google/uuid"

	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
)

// ProjectAudit returns the newest audit events for a project.
func (s *Service) ProjectAudit(ctx context.Context, projectID uuid.UUID, limit int32) ([]db.AuditEvent, error) {
	return s.Store.Queries().ListAuditEventsForProject(ctx, db.ListAuditEventsForProjectParams{TenantID: s.Tenant, ProjectID: &projectID, Limit: limit})
}
