package enhancepoints

import (
	"testing"
	"time"

	"raid-website/internal/validation"
)

func TestCanLock(t *testing.T) {
	if !CanLock(150) {
		t.Fatal("150 should allow")
	}
	if CanLock(149) {
		t.Fatal("149 should not")
	}
	if CanLock(0) {
		t.Fatal("0 should not")
	}
}

func TestExpireActiveOnEnd(t *testing.T) {
	now := time.Now().UTC()
	active := []ItemLock{
		{ID: 1, Status: validation.LockStatusActive, LockedAt: now},
		{ID: 2, Status: validation.LockStatusUsed, LockedAt: now},
	}
	exp := ExpireActiveOnEnd(active)
	if exp[0].Status != validation.LockStatusExpired || exp[0].ExpiredAt == nil {
		t.Fatal("active must expire")
	}
	if exp[1].Status != validation.LockStatusUsed || exp[1].ExpiredAt != nil {
		t.Fatal("used unchanged")
	}
}
