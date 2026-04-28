package sender

import (
	"context"
	"testing"

	"github.com/aibank/platform/services/notification-service/internal/domain"
)

func TestMockSender_AlwaysSucceeds(t *testing.T) {
	m := NewMockSender(domain.RecipientEmail)
	n := &domain.Notification{ID: "ntf_1", Recipient: "user@example.com"}
	if err := m.Send(context.Background(), n, Rendered{Subject: "S", Body: "B"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	rs := m.Records()
	if len(rs) != 1 || rs[0].NotificationID != "ntf_1" || rs[0].Subject != "S" {
		t.Fatalf("unexpected records: %+v", rs)
	}
	if m.Channel() != domain.RecipientEmail {
		t.Fatalf("expected email channel, got %s", m.Channel())
	}
}

func TestMockSender_Concurrent(t *testing.T) {
	m := NewMockSender(domain.RecipientSMS)
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(i int) {
			n := &domain.Notification{ID: "ntf_x", Recipient: "+7"}
			_ = m.Send(context.Background(), n, Rendered{})
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	if len(m.Records()) != 10 {
		t.Fatalf("expected 10 records, got %d", len(m.Records()))
	}
}

func TestSMTPStub_ReturnsNotImplemented(t *testing.T) {
	s := NewSMTPSender(SMTPConfig{Host: "localhost", Port: 25})
	err := s.Send(context.Background(),
		&domain.Notification{ID: "ntf_1", Recipient: "u@e"},
		Rendered{})
	if err == nil {
		t.Fatal("expected ErrNotImplemented from SMTP stub")
	}
}

func TestSMSStub_ReturnsNotImplemented(t *testing.T) {
	s := NewSMSStubSender("smsc")
	err := s.Send(context.Background(),
		&domain.Notification{ID: "ntf_1", Recipient: "+79991234567"},
		Rendered{})
	if err == nil {
		t.Fatal("expected ErrNotImplemented from SMS stub")
	}
}
