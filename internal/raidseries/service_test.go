package raidseries

import (
	"errors"
	"math"
	"testing"
	"time"
)

func instant(t *testing.T, raw string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestNextOccurrence(t *testing.T) {
	for _, tc := range []struct {
		name, start, now, clock, zone, want string
		days                                []int
		interval                            int64
		err                                 error
	}{
		{"same_week_multiple_days", "2026-10-05T18:00:00Z", "2026-10-06T10:00:00Z", "20:00", "Europe/Berlin", "2026-10-07T18:00:00Z", []int{3, 1}, 2, nil},
		{"skip_ineligible_week", "2026-10-05T18:00:00Z", "2026-10-12T10:00:00Z", "20:00", "Europe/Berlin", "2026-10-19T18:00:00Z", []int{1, 3}, 2, nil},
		{"skip_elapsed_cycles", "2026-10-05T18:00:00Z", "2026-11-04T20:00:00Z", "20:00", "Europe/Berlin", "2026-11-16T19:00:00Z", []int{1, 3}, 2, nil},
		{"strictly_after_now", "2026-10-05T18:00:00Z", "2026-10-07T18:00:00Z", "20:00", "Europe/Berlin", "2026-10-12T18:00:00Z", []int{1, 3}, 1, nil},
		{"after_future_current", "2026-10-07T18:00:00Z", "2026-10-04T10:00:00Z", "20:00", "Europe/Berlin", "2026-10-19T18:00:00Z", []int{1, 3}, 2, nil},
		{"sunday_uses_monday_anchor", "2026-10-11T18:00:00Z", "2026-10-11T19:00:00Z", "20:00", "Europe/Berlin", "2026-10-19T18:00:00Z", []int{0, 1}, 2, nil},
		{"local_date_not_utc_date", "2026-10-04T22:30:00Z", "2026-10-05T01:00:00Z", "00:30", "Europe/Berlin", "2026-10-18T22:30:00Z", []int{1}, 2, nil},
		{"spring_preserves_wall_time", "2026-03-22T19:00:00Z", "2026-03-22T20:00:00Z", "20:00", "Europe/Berlin", "2026-03-29T18:00:00Z", []int{0}, 1, nil},
		{"autumn_preserves_wall_time", "2026-10-18T18:00:00Z", "2026-10-18T20:00:00Z", "20:00", "Europe/Berlin", "2026-10-25T19:00:00Z", []int{0}, 1, nil},
		{"spring_gap", "2026-03-22T01:30:00Z", "2026-03-22T02:00:00Z", "02:30", "Europe/Berlin", "", []int{0}, 1, ErrLocalTimeNonexistent},
		{"autumn_fold", "2026-10-18T00:30:00Z", "2026-10-18T02:00:00Z", "02:30", "Europe/Berlin", "", []int{0}, 1, ErrLocalTimeAmbiguous},
		{"fold_partly_elapsed", "2026-10-18T00:30:00Z", "2026-10-25T00:45:00Z", "02:30", "Europe/Berlin", "", []int{0}, 1, ErrLocalTimeAmbiguous},
		{"elapsed_fold_skipped", "2026-10-18T00:30:00Z", "2026-10-25T02:00:00Z", "02:30", "Europe/Berlin", "2026-11-01T01:30:00Z", []int{0}, 1, nil},
		{"elapsed_gap_skipped", "2026-03-22T01:30:00Z", "2026-03-29T04:00:00Z", "02:30", "Europe/Berlin", "2026-04-05T00:30:00Z", []int{0}, 1, nil},
		{"half_hour_fold", "2026-03-29T01:45:00+11:00", "2026-03-29T02:00:00+11:00", "01:45", "Australia/Lord_Howe", "", []int{0}, 1, ErrLocalTimeAmbiguous},
		{"overflow_interval", "2026-10-05T20:00:00Z", "2026-10-05T21:00:00Z", "20:00", "UTC", "", []int{1}, math.MaxInt64, ErrScheduleRange},
		{"huge_interval_same_week", "2026-10-05T20:00:00Z", "2026-10-05T21:00:00Z", "20:00", "UTC", "2026-10-07T20:00:00Z", []int{1, 3}, math.MaxInt64, nil},
		{"large_valid_interval", "2026-10-05T20:00:00Z", "2026-10-05T21:00:00Z", "20:00", "UTC", "2028-09-04T20:00:00Z", []int{1}, 100, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Series{Name: "Explicit", RaidLeaderID: 1, Weekdays: tc.days, IntervalWeeks: tc.interval, StartTime: tc.clock, Timezone: tc.zone}
			got, err := NextOccurrence(s, instant(t, tc.start), instant(t, tc.now))
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("want %v got %v (%s)", tc.err, err, got)
				}
				return
			}
			if err != nil || !got.Equal(instant(t, tc.want)) {
				t.Fatalf("want %s got %s err=%v", tc.want, got.Format(time.RFC3339), err)
			}
		})
	}
}

func TestValidateRequiresExplicitConfiguration(t *testing.T) {
	valid := Series{Name: "Raid", RaidLeaderID: 1, Weekdays: []int{0, 6}, IntervalWeeks: 1, StartTime: "20:00", Timezone: "Europe/Berlin"}
	for _, mutate := range []func(*Series){
		func(s *Series) { s.Name = " " }, func(s *Series) { s.RaidLeaderID = 0 },
		func(s *Series) { s.Weekdays = nil }, func(s *Series) { s.Weekdays = []int{-1} },
		func(s *Series) { s.Weekdays = []int{7} }, func(s *Series) { s.Weekdays = []int{1, 1} },
		func(s *Series) { s.IntervalWeeks = 0 }, func(s *Series) { s.IntervalWeeks = -1 },
		func(s *Series) { s.StartTime = "" }, func(s *Series) { s.StartTime = "2:00" },
		func(s *Series) { s.StartTime = "24:00" }, func(s *Series) { s.StartTime = "12:60" },
		func(s *Series) { s.Timezone = "" }, func(s *Series) { s.Timezone = "Local" },
		func(s *Series) { s.Timezone = "unknown/place" },
	} {
		copy := valid
		mutate(&copy)
		if !errors.Is(Validate(&copy), ErrInvalid) {
			t.Fatalf("accepted invalid series: %+v", copy)
		}
	}
	if err := Validate(&valid); err != nil {
		t.Fatal(err)
	}
}
