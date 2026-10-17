package raidseries

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

var (
	ErrNotFound               = errors.New("raid series not found")
	ErrInvalid                = errors.New("invalid raid series")
	ErrLeaderNotFound         = errors.New("raid series leader not found")
	ErrSeriesRequired         = errors.New("raid series required")
	ErrScheduledStartRequired = errors.New("scheduled start required")
	ErrLocalTimeNonexistent   = errors.New("local occurrence does not exist")
	ErrLocalTimeAmbiguous     = errors.New("local occurrence is ambiguous")
	ErrScheduleRange          = errors.New("schedule exceeds representable calendar range")
)

// Validate rejects incomplete configuration; there are no guild defaults.
func Validate(s *Series) error {
	if strings.TrimSpace(s.Name) == "" || s.RaidLeaderID <= 0 || len(s.Weekdays) == 0 || s.IntervalWeeks <= 0 {
		return ErrInvalid
	}
	seen := map[int]bool{}
	for _, day := range s.Weekdays {
		if day < 0 || day > 6 || seen[day] {
			return ErrInvalid
		}
		seen[day] = true
	}
	if len(s.StartTime) != 5 {
		return ErrInvalid
	}
	if _, err := time.Parse("15:04", s.StartTime); err != nil {
		return ErrInvalid
	}
	if s.Timezone == "" || s.Timezone == "Local" {
		return ErrInvalid
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return ErrInvalid
	}
	return nil
}

// Service validates configuration before repository writes.
type Service struct{ repo *Repo }

func NewService(repo *Repo) *Service { return &Service{repo: repo} }
func (s *Service) Save(ctx context.Context, series *Series) error {
	result, err := s.Configure(ctx, series.ID, Changes{Name: &series.Name, RaidLeaderID: &series.RaidLeaderID, Weekdays: &series.Weekdays, IntervalWeeks: &series.IntervalWeeks, StartTime: &series.StartTime, Timezone: &series.Timezone, CatalogInstanceID: series.CatalogInstanceID, RaidSize: series.RaidSize})
	if err == nil {
		*series = *result
	}
	return err
}

// Changes contains only explicitly supplied configuration fields.
type Changes struct {
	// ActorID is the player making the change; it is written to the audit log.
	ActorID           int64
	CatalogInstanceID *string
	RaidSize          *int
	Name              *string
	NameTemplate      *string
	NextRaidNumber    *int
	RaidLeaderID      *int64
	Weekdays          *[]int
	IntervalWeeks     *int64
	StartTime         *string
	Timezone          *string
}

// Configure creates or patches configuration under the same writer reservation as archival.
func (s *Service) Configure(ctx context.Context, id int64, changes Changes) (*Series, error) {
	var result *Series
	err := s.repo.InTransaction(ctx, func(repo *Repo) error {
		series := new(Series)
		var before *Series
		if id != 0 {
			var err error
			series, err = repo.Get(ctx, id)
			if err != nil {
				return err
			}
			snapshot := *series
			before = &snapshot
		}
		if changes.Name != nil {
			series.Name = *changes.Name
		}
		if changes.NameTemplate != nil {
			series.NameTemplate = *changes.NameTemplate
		}
		if changes.NextRaidNumber != nil {
			series.NextRaidNumber = *changes.NextRaidNumber
		}
		if changes.RaidLeaderID != nil {
			series.RaidLeaderID = *changes.RaidLeaderID
		}
		if changes.Weekdays != nil {
			series.Weekdays = *changes.Weekdays
		}
		if changes.IntervalWeeks != nil {
			series.IntervalWeeks = *changes.IntervalWeeks
		}
		if changes.StartTime != nil {
			series.StartTime = *changes.StartTime
		}
		if changes.Timezone != nil {
			series.Timezone = *changes.Timezone
		}
		if err := Validate(series); err != nil {
			return err
		}
		if changes.CatalogInstanceID != nil {
			series.CatalogInstanceID = changes.CatalogInstanceID
		}
		if changes.RaidSize != nil {
			series.RaidSize = changes.RaidSize
		}
		if err := repo.Catalog().ValidateAssociation(ctx, series.CatalogInstanceID, series.RaidSize); err != nil {
			return err
		}
		if err := repo.RequireLeader(ctx, series.RaidLeaderID); err != nil {
			return err
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if id == 0 {
			series.CreatedAt = now
		}
		series.UpdatedAt = now
		if err := repo.Save(ctx, series); err != nil {
			return err
		}
		action := "update"
		if id == 0 {
			action, before = "create", nil
		}
		if err := repo.Audit(ctx, changes.ActorID, series.ID, action, before, series); err != nil {
			return err
		}
		result = series
		return nil
	})
	return result, err
}

// NextOccurrence anchors eligible weeks to the current raid's local Monday.
// Calendar arithmetic uses UTC civil dates, never elapsed 168-hour DST weeks.
func NextOccurrence(s *Series, scheduledStart, now time.Time) (time.Time, error) {
	if err := Validate(s); err != nil {
		return time.Time{}, err
	}
	location, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return time.Time{}, errors.Join(ErrInvalid, err)
	}
	clock, err := time.Parse("15:04", s.StartTime)
	if err != nil {
		return time.Time{}, errors.Join(ErrInvalid, err)
	}
	threshold := now
	if scheduledStart.After(threshold) {
		threshold = scheduledStart
	}
	local := scheduledStart.In(location)
	if local.Year() < 0 || local.Year() > 9999 || threshold.In(location).Year() > 9999 {
		return time.Time{}, ErrScheduleRange
	}
	anchor := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	anchor = anchor.AddDate(0, 0, -(int(local.Weekday())+6)%7)
	localThreshold := threshold.In(location)
	thresholdDate := time.Date(localThreshold.Year(), localThreshold.Month(), localThreshold.Day(), 0, 0, 0, 0, time.UTC)
	weeks := (thresholdDate.Unix() - anchor.Unix()) / 86400 / 7
	if weeks < 0 {
		weeks = 0
	}
	cycle := weeks / s.IntervalWeeks
	days := append([]int(nil), s.Weekdays...)
	sort.Slice(days, func(i, j int) bool { return (days[i]+6)%7 < (days[j]+6)%7 })
	for attempt := 0; attempt < 2; attempt++ {
		// RFC3339 storage requires a four-digit year. Check before AddDate's int conversion.
		last := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
		maxDays := (last.Unix() - anchor.Unix()) / 86400
		if cycle > 0 && s.IntervalWeeks > maxDays/7/cycle {
			return time.Time{}, ErrScheduleRange
		}
		offsetDays := int64(0)
		if cycle > 0 {
			offsetDays = cycle * s.IntervalWeeks * 7
		}
		week := anchor.AddDate(0, 0, int(offsetDays))
		for _, day := range days {
			date := week.AddDate(0, 0, (day+6)%7)
			if date.Before(thresholdDate) {
				continue
			}
			if date.Year() > 9999 {
				return time.Time{}, ErrScheduleRange
			}
			wall := time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, time.UTC)
			matches := localOccurrences(wall, location)
			if len(matches) == 0 {
				thresholdWall := time.Date(localThreshold.Year(), localThreshold.Month(), localThreshold.Day(), localThreshold.Hour(), localThreshold.Minute(), localThreshold.Second(), localThreshold.Nanosecond(), time.UTC)
				if !wall.After(thresholdWall) {
					continue
				}
				return time.Time{}, ErrLocalTimeNonexistent
			}
			future := false
			for _, candidate := range matches {
				if candidate.After(threshold) {
					future = true
				}
			}
			if !future {
				continue
			}
			if len(matches) != 1 {
				return time.Time{}, ErrLocalTimeAmbiguous
			}
			if matches[0].Year() < 0 || matches[0].Year() > 9999 {
				return time.Time{}, ErrScheduleRange
			}
			return matches[0].UTC(), nil
		}
		cycle++
	}
	return time.Time{}, ErrScheduleRange
}

// Enumerate offsets around the civil date to detect both sides of a DST fold.
func localOccurrences(wall time.Time, location *time.Location) []time.Time {
	var matches []time.Time
	seen := map[int]bool{}
	end := wall.Add(48 * time.Hour)
	for cursor := wall.Add(-48 * time.Hour); cursor.Before(end); {
		local := cursor.In(location)
		_, offset := local.Zone()
		if !seen[offset] {
			seen[offset] = true
			candidate := wall.Add(-time.Duration(offset) * time.Second)
			actual := candidate.In(location)
			if actual.Year() == wall.Year() && actual.Month() == wall.Month() && actual.Day() == wall.Day() && actual.Hour() == wall.Hour() && actual.Minute() == wall.Minute() {
				matches = append(matches, candidate)
			}
		}
		_, bound := local.ZoneBounds()
		if bound.IsZero() || !bound.After(cursor) {
			break
		}
		cursor = bound
	}
	return matches
}

// EvaluateName renders a naming template with the given counter and raid date.
// Supported variables: {number}, {name}, {date}, {week}.
func EvaluateName(template string, number int, seriesName string, raidDate time.Time) string {
	result := template
	result = strings.ReplaceAll(result, "{number}", strconv.Itoa(number))
	result = strings.ReplaceAll(result, "{name}", seriesName)
	result = strings.ReplaceAll(result, "{date}", raidDate.Format("2006-01-02"))
	_, week := raidDate.ISOWeek()
	result = strings.ReplaceAll(result, "{week}", strconv.Itoa(week))
	return result
}
