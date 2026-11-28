package scheduledtask

import (
	"fmt"
	"slices"
	"time"
	_ "time/tzdata"
)

const maxIntervalMinutes = 999 * 24 * 60

func cloneScheduleDetail(detail *ScheduleDetail) *ScheduleDetail {
	if detail == nil {
		return nil
	}
	copy := *detail
	copy.Weekdays = slices.Clone(detail.Weekdays)
	copy.MonthDays = slices.Clone(detail.MonthDays)
	return &copy
}

func validateSchedule(item *Task) error {
	detail := item.ScheduleDetail
	if item.Recurrence == RecurrenceOnce {
		item.StartsAt = nil
		item.ScheduleDetail = nil
		if detail != nil {
			item.ExpiresAt = nil
		}
	}
	if item.StartsAt != nil && item.ExpiresAt != nil && item.StartsAt.After(*item.ExpiresAt) {
		return fmt.Errorf("starts_at must not be after expires_at")
	}
	if !item.NextRunAt.IsZero() && !started(*item, item.NextRunAt) {
		return fmt.Errorf("next_run_at must not be before starts_at")
	}
	if item.Enabled && !item.NextRunAt.IsZero() && expired(*item, item.NextRunAt) {
		return fmt.Errorf("expires_at must not be before next_run_at")
	}

	switch item.Recurrence {
	case RecurrenceOnce:
		return nil
	case RecurrenceInterval:
		if detail == nil || detail.IntervalMinutes < 1 || detail.IntervalMinutes > maxIntervalMinutes {
			return fmt.Errorf("invalid schedule interval_minutes")
		}
		return nil
	case RecurrenceWeekly, RecurrenceBiweekly:
		if (detail == nil && item.Recurrence == RecurrenceBiweekly) || (detail != nil && !validDays(detail.Weekdays, 0, 6)) {
			return fmt.Errorf("invalid schedule weekdays")
		}
	case RecurrenceMonthly:
		if detail != nil && !validDays(detail.MonthDays, 1, 31) {
			return fmt.Errorf("invalid schedule month_days")
		}
	case RecurrenceYearly:
		if detail == nil || detail.Month < 1 || detail.Month > 12 || detail.Day < 1 || detail.Day > 31 ||
			time.Date(2000, time.Month(detail.Month), detail.Day, 0, 0, 0, 0, time.UTC).Month() != time.Month(detail.Month) {
			return fmt.Errorf("invalid yearly schedule date")
		}
	}
	if detail == nil {
		return nil
	}
	location, err := scheduleLocation(detail)
	if err != nil {
		return fmt.Errorf("invalid schedule time_zone")
	}
	clock, err := time.Parse("15:04", detail.Clock)
	if detail.Clock != "" && err != nil {
		return fmt.Errorf("invalid schedule clock")
	}
	if !item.NextRunAt.IsZero() {
		next := item.NextRunAt.In(location)
		if !matchesDate(*item, next) ||
			(detail.Clock != "" && (next.Hour() != clock.Hour() || next.Minute() != clock.Minute())) {
			return fmt.Errorf("next_run_at does not match schedule")
		}
	}
	return nil
}

func validDays(days []int, min, max int) bool {
	if len(days) == 0 || len(days) > max-min+1 {
		return false
	}
	for _, day := range days {
		if day < min || day > max {
			return false
		}
	}
	return true
}

func started(item Task, at time.Time) bool {
	return item.StartsAt == nil || !at.Before(*item.StartsAt)
}

func scheduleLocation(detail *ScheduleDetail) (*time.Location, error) {
	if detail.TimeZone == "" {
		return time.Local, nil
	}
	return time.LoadLocation(detail.TimeZone)
}

func nextStructuredRun(item Task, from time.Time) (time.Time, bool) {
	detail := item.ScheduleDetail
	if item.Recurrence == RecurrenceInterval {
		step := time.Duration(detail.IntervalMinutes) * time.Minute
		if step <= 0 {
			return time.Time{}, false
		}
		if item.NextRunAt.After(from) {
			return item.NextRunAt, true
		}
		// Preserve the original cadence when the scheduler misses an occurrence.
		return from.Add(step - from.Sub(item.NextRunAt)%step), true
	}
	location, err := scheduleLocation(detail)
	if err != nil {
		return time.Time{}, false
	}
	anchor := item.NextRunAt.In(location)
	hour, minute, second := anchor.Clock()
	if detail.Clock != "" {
		clock, err := time.Parse("15:04", detail.Clock)
		if err != nil {
			return time.Time{}, false
		}
		hour, minute, second = clock.Clock()
	}

	year, month, day := from.In(location).Date()
	date := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	// Eight years cover leap-day schedules across a non-leap century.
	for offset := 0; offset <= 8*366; offset++ {
		year, month, day := date.Date()
		if matchesDate(item, date) && (item.Recurrence != RecurrenceBiweekly || sameBiweeklyPhase(anchor, date)) {
			candidate := time.Date(year, month, day, hour, minute, second, 0, location)
			if candidate.Hour() == hour && candidate.Minute() == minute && candidate.After(from) && started(item, candidate) {
				return candidate, true
			}
		}
		date = date.AddDate(0, 0, 1)
	}
	return time.Time{}, false
}

func matchesDate(item Task, date time.Time) bool {
	detail := item.ScheduleDetail
	switch item.Recurrence {
	case RecurrenceDaily:
		return true
	case RecurrenceWeekly, RecurrenceBiweekly:
		return slices.Contains(detail.Weekdays, int(date.Weekday()))
	case RecurrenceMonthly:
		return slices.Contains(detail.MonthDays, date.Day())
	case RecurrenceYearly:
		return int(date.Month()) == detail.Month && date.Day() == detail.Day
	default:
		return false
	}
}

// Biweekly schedules use Monday-based weeks. The first occurrence fixes the phase.
func sameBiweeklyPhase(anchor, date time.Time) bool {
	year, month, day := anchor.Date()
	base := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	base = base.AddDate(0, 0, -(int(base.Weekday())+6)%7)
	week := date.AddDate(0, 0, -(int(date.Weekday())+6)%7)
	return (week.Unix()-base.Unix())%(14*86400) == 0
}
