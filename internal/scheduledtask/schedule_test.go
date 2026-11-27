package scheduledtask

import (
	"context"
	"testing"
	"time"
)

func TestStructuredSchedulesAdvanceAfterRestart(t *testing.T) {
	parse := func(value string) time.Time {
		t.Helper()
		at, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}
		return at
	}
	tests := []struct {
		name       string
		detail     ScheduleDetail
		first, now string
		next       string
	}{
		{RecurrenceInterval, ScheduleDetail{IntervalMinutes: 90}, "2026-07-06T09:00:00Z", "2026-07-06T12:20:00Z", "2026-07-06T13:30:00Z"},
		{RecurrenceDaily, ScheduleDetail{Clock: "09:00", TimeZone: "America/New_York"}, "2026-03-07T14:00:00Z", "2026-03-07T14:00:00Z", "2026-03-08T13:00:00Z"},
		{RecurrenceWeekly, ScheduleDetail{Weekdays: []int{0, 4}, TimeZone: "UTC"}, "2026-07-09T09:00:00Z", "2026-07-09T09:00:00Z", "2026-07-12T09:00:00Z"},
		{RecurrenceBiweekly, ScheduleDetail{Weekdays: []int{1, 4}, TimeZone: "UTC"}, "2026-07-06T09:00:00Z", "2026-07-06T09:00:00Z", "2026-07-09T09:00:00Z"},
		{RecurrenceBiweekly, ScheduleDetail{Weekdays: []int{1, 4}, TimeZone: "UTC"}, "2026-07-09T09:00:00Z", "2026-07-13T10:00:00Z", "2026-07-20T09:00:00Z"},
		{RecurrenceMonthly, ScheduleDetail{MonthDays: []int{31}, TimeZone: "UTC"}, "2026-01-31T09:00:00Z", "2026-01-31T09:00:00Z", "2026-03-31T09:00:00Z"},
		{RecurrenceYearly, ScheduleDetail{Month: 2, Day: 29, TimeZone: "UTC"}, "2096-02-29T09:00:00Z", "2096-02-29T09:00:00Z", "2104-02-29T09:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.now, func(t *testing.T) {
			first, now, next := parse(tt.first), parse(tt.now), parse(tt.next)
			starts, expires := first.Add(-time.Hour), next.Add(-time.Second)
			svc := newTestService(t, first.Add(-2*time.Hour))
			item, err := svc.Create(CreateInput{
				Title: "Report", AgentID: "u-worker", Prompt: "Generate report",
				Recurrence: tt.name, ScheduleDetail: &tt.detail, FirstRunAt: first,
				StartsAt: &starts, ExpiresAt: &expires, Enabled: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			clock := starts.Add(-time.Minute)
			restarted, err := NewService(svc.store, nil, WithNowFunc(func() time.Time { return clock }))
			if err != nil {
				t.Fatal(err)
			}
			if got := restarted.List()[0]; got.StartsAt == nil || !got.StartsAt.Equal(starts) ||
				got.ExpiresAt == nil || !got.ExpiresAt.Equal(expires) || got.ScheduleDetail == nil {
				t.Fatalf("schedule was not persisted: %+v", got)
			}
			if runs := restarted.TriggerDue(context.Background()); len(runs) != 0 {
				t.Fatalf("triggered before starts_at: %+v", runs)
			}
			clock = now
			if runs := restarted.TriggerDue(context.Background()); len(runs) != 1 || runs[0].ScheduledTaskID != item.ID {
				t.Fatalf("expected one run: %+v", runs)
			}
			if got := restarted.List()[0]; !got.NextRunAt.Equal(next) || got.Enabled {
				t.Fatalf("next run = %v, enabled = %v; want %v and disabled after expiration", got.NextRunAt, got.Enabled, next)
			}
		})
	}
}

func TestChangingToOnceClearsValidity(t *testing.T) {
	now := time.Now()
	starts, first, expires := now.Add(time.Hour), now.Add(2*time.Hour), now.Add(3*time.Hour)
	svc := newTestService(t, now)
	task, err := svc.Create(CreateInput{
		Title: "Report", AgentID: "u-worker", Prompt: "Generate report",
		Recurrence: RecurrenceDaily, ScheduleDetail: &ScheduleDetail{},
		FirstRunAt: first, StartsAt: &starts, ExpiresAt: &expires, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	once := RecurrenceOnce
	next := now.Add(4 * time.Hour)
	expiresPtr := &expires
	got, err := svc.Update(UpdateInput{ID: task.ID, Recurrence: &once, NextRunAt: &next, ExpiresAt: &expiresPtr})
	if err != nil {
		t.Fatal(err)
	}
	if got.ScheduleDetail != nil || got.StartsAt != nil || got.ExpiresAt != nil {
		t.Fatalf("once retained recurring fields: %+v", got)
	}
}
