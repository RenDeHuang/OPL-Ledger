package usage

import (
	"errors"
	"time"
)

var ErrRequestQuotaExceeded = errors.New("request_quota_exceeded")

type RequestQuota struct {
	Limit           *int64    `json:"limit,omitempty"`
	Used            int64     `json:"used"`
	WindowLimit     *int64    `json:"windowLimit,omitempty"`
	WindowUsed      int64     `json:"windowUsed"`
	WindowSeconds   int64     `json:"windowSeconds,omitempty"`
	WindowStartedAt time.Time `json:"windowStartedAt,omitempty"`
}

func IncrementOptionalRequestQuota(quota *RequestQuota, units int64, now time.Time) (*RequestQuota, error) {
	if quota == nil {
		return nil, nil
	}
	next, err := IncrementRequestQuota(*quota, units, now)
	if err != nil {
		return nil, err
	}
	return &next, nil
}

func IncrementRequestQuota(quota RequestQuota, units int64, now time.Time) (RequestQuota, error) {
	amount := units
	if amount <= 0 {
		return quota, nil
	}
	next := quota
	if next.Limit != nil && *next.Limit >= 0 && next.Used+amount > *next.Limit {
		return RequestQuota{}, ErrRequestQuotaExceeded
	}
	if requestQuotaWindowExpired(next, now) {
		next.WindowUsed = 0
		next.WindowStartedAt = now
	}
	if next.WindowLimit != nil {
		if *next.WindowLimit >= 0 && next.WindowUsed+amount > *next.WindowLimit {
			return RequestQuota{}, ErrRequestQuotaExceeded
		}
		next.WindowUsed += amount
		if next.WindowStartedAt.IsZero() {
			next.WindowStartedAt = now
		}
	}
	next.Used += amount
	return next, nil
}

func requestQuotaWindowExpired(quota RequestQuota, now time.Time) bool {
	if quota.WindowSeconds <= 0 || quota.WindowStartedAt.IsZero() {
		return false
	}
	return !now.Before(quota.WindowStartedAt.Add(time.Duration(quota.WindowSeconds) * time.Second))
}
