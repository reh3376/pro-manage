package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/reh3376/pro-manage/services/pm-core/internal/acl"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
)

// Live refresh (FSD §05 "Live updates via SSE at /api/v1/projects/{id}/stream",
// §09 "the frontend fetches the fresh tile"). The streams carry hints, never
// payloads: which subject changed and how, read from the audit log, which every
// mutation writes in its own transaction (so a hint is never sent for something
// that did not commit, and pm-workers' changes arrive the same way). The
// subscriber re-fetches through the normal routes, where the §04 ACL applies.
//
// Project stream: events of one project, gated on the overview read. Inbox
// stream: workflow and CAR events across the projects the caller can see, the
// visible set refreshed every 30 s. Both poll the audit sequence every 2 s and
// send a comment heartbeat every 15 s.
func (a *API) mountStreams(mux *http.ServeMux, wrap func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/v1/projects/{project_id}/stream", wrap(a.projectStream))
	mux.Handle("GET /api/v1/inbox/stream", wrap(a.inboxStream))
}

// ChangeEvent is the SSE payload.
type ChangeEvent struct {
	Seq         int64  `json:"seq"`
	At          string `json:"at"`
	ProjectID   string `json:"project_id,omitempty"`
	ActionType  string `json:"action_type"`
	SubjectType string `json:"subject_type"`
	SubjectID   string `json:"subject_id"`
}

func changeEvent(e db.AuditEvent) ChangeEvent {
	ev := ChangeEvent{Seq: e.Seq, At: e.At.UTC().Format(time.RFC3339), ActionType: e.ActionType, SubjectType: e.SubjectType, SubjectID: e.SubjectID}
	if e.ProjectID != nil {
		ev.ProjectID = e.ProjectID.String()
	}
	return ev
}

// inboxRelevant: the action kinds that can change what an inbox shows.
func inboxRelevant(action string) bool {
	return strings.HasPrefix(action, "car.") || strings.HasPrefix(action, "workflow.") || strings.HasPrefix(action, "stage.")
}

const (
	streamPoll       = 2 * time.Second
	streamHeartbeat  = 15 * time.Second
	streamVisibleTTL = 30 * time.Second
	streamBatch      = int32(200)
)

func (a *API) projectStream(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, ok := a.authorize(w, r, id, acl.Overview, acl.Read); !ok {
		return
	}
	a.stream(w, r, func(e db.AuditEvent) bool { return e.ProjectID != nil && *e.ProjectID == id })
}

func (a *API) inboxStream(w http.ResponseWriter, r *http.Request) {
	actor := actorFrom(r.Context())
	visible := map[uuid.UUID]bool{}
	var refreshed time.Time
	refresh := func() {
		if time.Since(refreshed) < streamVisibleTTL {
			return
		}
		refreshed = time.Now()
		projects, err := a.Domain.ListVisibleProjects(r.Context(), actor, nil)
		if err != nil {
			return
		}
		visible = map[uuid.UUID]bool{}
		for _, p := range projects {
			visible[p.ProjectID] = true
		}
	}
	refresh()
	a.stream(w, r, func(e db.AuditEvent) bool {
		if !inboxRelevant(e.ActionType) || e.ProjectID == nil {
			return false
		}
		refresh()
		return visible[*e.ProjectID]
	})
}

// stream runs the SSE loop until the client goes away.
func (a *API) stream(w http.ResponseWriter, r *http.Request, want func(db.AuditEvent) bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "streaming not supported")
		return
	}
	ctx := r.Context()
	q := a.Domain.Store.Queries()
	seq, err := q.MaxAuditSeq(ctx, a.Domain.Tenant)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, ": connected seq=%d\n\n", seq)
	flusher.Flush()

	poll := time.NewTicker(streamPoll)
	defer poll.Stop()
	beat := time.NewTicker(streamHeartbeat)
	defer beat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-beat.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case <-poll.C:
			rows, err := q.ListAuditEventsInOrder(ctx, db.ListAuditEventsInOrderParams{TenantID: a.Domain.Tenant, Seq: seq, Limit: streamBatch})
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				a.Log.Warn("stream poll", "err", err)
				continue
			}
			sent := false
			for _, e := range rows {
				seq = e.Seq
				if !want(e) {
					continue
				}
				b, _ := json.Marshal(changeEvent(e))
				fmt.Fprintf(w, "id: %d\nevent: change\ndata: %s\n\n", e.Seq, b)
				sent = true
			}
			if sent {
				flusher.Flush()
			}
		}
	}
}
