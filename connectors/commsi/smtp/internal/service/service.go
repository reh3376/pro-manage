// Package service implements pm.connector.v1.ConnectorService and
// pm.connector.v1.CommsiService over the SMTP sender.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	netmail "net/mail"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/reh3376/pro-manage/connectors/commsi/smtp/internal/mail"
	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

// Manifest mirrors connector.yaml.
type Manifest struct {
	Module, Type, Code, Version, Contract string
	Capabilities                          []string
}

// DefaultManifest is the module's.
var DefaultManifest = Manifest{Module: "Commsi_SMTP", Type: "Commsi", Code: "SMTP", Contract: "pm.connector.v1.CommsiService",
	Capabilities: []string{"notification_send", "digest_send", "dedupe_window", "deep_links"}}

// Service holds the sender.
type Service struct {
	connectorv1.UnimplementedConnectorServiceServer
	connectorv1.UnimplementedCommsiServiceServer
	Sender   *mail.Sender
	Manifest Manifest
	Log      *slog.Logger
}

func (s *Service) HealthCheck(ctx context.Context, _ *connectorv1.HealthCheckRequest) (*connectorv1.HealthCheckResponse, error) {
	resp := &connectorv1.HealthCheckResponse{Status: connectorv1.HealthCheckResponse_STATUS_SERVING, Checks: map[string]string{}, Detail: "relay " + s.Sender.Addr()}
	if err := s.Sender.Probe(ctx); err != nil {
		resp.Status = connectorv1.HealthCheckResponse_STATUS_DEGRADED
		resp.Detail = "smtp relay not answering"
		resp.Checks["smtp"] = err.Error()
		return resp, nil
	}
	resp.Checks["smtp"] = "ok"
	return resp, nil
}

func (s *Service) Capabilities(_ context.Context, _ *connectorv1.CapabilitiesRequest) (*connectorv1.CapabilitiesResponse, error) {
	m := s.Manifest
	return &connectorv1.CapabilitiesResponse{Module: m.Module, Type: m.Type, Code: m.Code, Version: m.Version, Contract: m.Contract, Capabilities: m.Capabilities, Tiers: []string{}}, nil
}

func recipients(rs []*connectorv1.Recipient) ([]netmail.Address, error) {
	out := make([]netmail.Address, 0, len(rs))
	for _, r := range rs {
		if r.GetEmail() == "" {
			return nil, status.Errorf(codes.InvalidArgument, "recipient %s has no email", r.GetUserId())
		}
		if _, err := netmail.ParseAddress(r.GetEmail()); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "recipient %s: bad email %q", r.GetUserId(), r.GetEmail())
		}
		name := r.GetName()
		if name == "" {
			name = r.GetUserId()
		}
		out = append(out, netmail.Address{Name: name, Address: r.GetEmail()})
	}
	if len(out) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one recipient is required")
	}
	return out, nil
}

func (s *Service) SendNotification(ctx context.Context, req *connectorv1.SendNotificationRequest) (*connectorv1.SendNotificationResponse, error) {
	if req.GetKind() == "" || req.GetSubject() == "" {
		return nil, status.Error(codes.InvalidArgument, "kind and subject are required")
	}
	to, err := recipients(req.GetRecipients())
	if err != nil {
		return nil, err
	}
	text := req.GetBodyText()
	if req.GetDueAt() != nil {
		text = strings.TrimRight(text, "\n") + "\nDue: " + req.GetDueAt().AsTime().UTC().Format(time.RFC1123) + "\n"
	}
	res, err := s.Sender.Send(ctx, mail.Message{To: to, Subject: req.GetSubject(), Text: text, HTML: req.GetBodyHtml(), DeepLink: req.GetDeepLink(), DedupeKey: req.GetDedupeKey(),
		Headers: map[string]string{"X-PM-Kind": req.GetKind(), "X-PM-Subject": req.GetSubjectType() + ":" + req.GetSubjectId()}})
	if err != nil {
		return nil, grpcErr(err)
	}
	if res.Status == "sent" {
		s.Log.Info("notification sent", "kind", req.GetKind(), "subject", req.GetSubjectType()+":"+req.GetSubjectId(), "recipients", len(to), "message_id", res.MessageID)
	}
	return &connectorv1.SendNotificationResponse{DeliveryId: res.MessageID, Channel: "smtp", Status: res.Status, Detail: res.Detail}, nil
}

func (s *Service) SendDigest(ctx context.Context, req *connectorv1.SendDigestRequest) (*connectorv1.SendDigestResponse, error) {
	if req.GetRecipient() == nil {
		return nil, status.Error(codes.InvalidArgument, "recipient is required")
	}
	to, err := recipients([]*connectorv1.Recipient{req.GetRecipient()})
	if err != nil {
		return nil, err
	}
	period := req.GetPeriod()
	if period == "" {
		period = "daily"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Your %s pro_manage digest: %d open item(s).\n\n", period, len(req.GetItems()))
	for _, it := range req.GetItems() {
		fmt.Fprintf(&b, "- [%s] %s", it.GetKind(), it.GetTitle())
		if it.GetDueAt() != nil {
			fmt.Fprintf(&b, " (due %s)", it.GetDueAt().AsTime().UTC().Format("2006-01-02 15:04 UTC"))
		}
		if it.GetDeepLink() != "" {
			fmt.Fprintf(&b, "\n  %s", it.GetDeepLink())
		}
		b.WriteString("\n")
	}
	res, err := s.Sender.Send(ctx, mail.Message{To: to, Subject: fmt.Sprintf("pro_manage %s digest: %d open item(s)", period, len(req.GetItems())), Text: b.String(), DeepLink: "/inbox", DedupeKey: req.GetDedupeKey(), Headers: map[string]string{"X-PM-Kind": "digest." + period}})
	if err != nil {
		return nil, grpcErr(err)
	}
	return &connectorv1.SendDigestResponse{DeliveryId: res.MessageID, Channel: "smtp", Status: res.Status, Detail: res.Detail}, nil
}

// OnDeliveryReceipt: SMTP has no receipt channel the module consumes yet (bounces arrive by mail).
func (s *Service) OnDeliveryReceipt(_ context.Context, req *connectorv1.OnDeliveryReceiptRequest) (*connectorv1.OnDeliveryReceiptResponse, error) {
	return &connectorv1.OnDeliveryReceiptResponse{Accepted: false, Detail: "smtp delivery receipts are not consumed by this module"}, nil
}

func grpcErr(err error) error {
	switch {
	case errors.Is(err, mail.ErrRejected):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, mail.ErrUnavailable):
		return status.Error(codes.Unavailable, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
