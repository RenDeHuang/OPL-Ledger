package usage

import (
	"errors"
	"testing"
	"time"
)

func TestIncrementQuotaRejectsTotalLimitExceeded(t *testing.T) {
	quota := RequestQuota{Limit: int64Ptr(2), Used: 2}

	_, err := IncrementRequestQuota(quota, 1, time.Now())

	if !errors.Is(err, ErrRequestQuotaExceeded) {
		t.Fatalf("expected quota exceeded, got %v", err)
	}
	if quota.Used != 2 {
		t.Fatalf("quota mutated on rejection: %+v", quota)
	}
}

func TestIncrementQuotaRejectsWindowLimitExceeded(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	quota := RequestQuota{
		WindowLimit:     int64Ptr(2),
		WindowUsed:      2,
		WindowSeconds:   60,
		WindowStartedAt: now.Add(-30 * time.Second),
	}

	_, err := IncrementRequestQuota(quota, 1, now)

	if !errors.Is(err, ErrRequestQuotaExceeded) {
		t.Fatalf("expected quota exceeded, got %v", err)
	}
}

func TestIncrementQuotaResetsExpiredWindow(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	quota := RequestQuota{
		WindowLimit:     int64Ptr(2),
		WindowUsed:      2,
		WindowSeconds:   60,
		WindowStartedAt: now.Add(-90 * time.Second),
	}

	next, err := IncrementRequestQuota(quota, 1, now)
	if err != nil {
		t.Fatalf("increment quota: %v", err)
	}
	if next.WindowUsed != 1 {
		t.Fatalf("window used = %d", next.WindowUsed)
	}
	if !next.WindowStartedAt.Equal(now) {
		t.Fatalf("window started at = %s", next.WindowStartedAt)
	}
}

func TestIncrementQuotaReturnsNilWhenQuotaAbsent(t *testing.T) {
	next, err := IncrementOptionalRequestQuota(nil, 1, time.Now())
	if err != nil {
		t.Fatalf("increment nil quota: %v", err)
	}
	if next != nil {
		t.Fatalf("expected nil quota, got %+v", next)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
