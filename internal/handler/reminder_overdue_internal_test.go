package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/calnode/calnode/internal/db"
)

func reminderHandler(t *testing.T) *Handler {
	t.Helper()
	database, err := db.Open("sqlite://:memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return New(database, slog.Default())
}

// reminderHours lists hours_before of every pending reminder job for bookingID.
func reminderHours(t *testing.T, h *Handler, bookingID string) []int {
	t.Helper()
	rows, err := h.db.QueryContext(context.Background(),
		`SELECT payload FROM jobs WHERE type = 'reminder.send' AND json_extract(payload, '$.booking_id') = ?`, bookingID)
	if err != nil {
		t.Fatalf("query jobs: %v", err)
	}
	defer rows.Close()
	var hours []int
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var p struct {
			HoursBefore int `json:"hours_before"`
		}
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			t.Fatalf("payload %q: %v", payload, err)
		}
		hours = append(hours, p.HoursBefore)
	}
	sort.Ints(hours)
	return hours
}

// A booking made 20h ahead is already inside the 24h window: that reminder would
// arrive seconds after the confirmation, so it is skipped; the 1h one still fires.
func TestEnqueueReminder_skipsOverdueKeepsFuture(t *testing.T) {
	h := reminderHandler(t)
	start := time.Now().UTC().Add(20 * time.Hour)
	for _, hb := range []int{24, 1} {
		if err := h.enqueueReminder(context.Background(), "b-1", start, hb); err != nil {
			t.Fatalf("enqueueReminder(%d): %v", hb, err)
		}
	}
	if got := reminderHours(t, h, "b-1"); len(got) != 1 || got[0] != 1 {
		t.Errorf("reminders = %v; want [1]", got)
	}
}

func TestReplaceReminderJobs_skipsOverdue(t *testing.T) {
	h := reminderHandler(t)
	ctx := context.Background()
	// No explicit list for this event type → the default single 24h reminder.
	if err := h.replaceReminderJobs(ctx, "b-2", "no-such-event-type", time.Now().UTC().Add(20*time.Hour)); err != nil {
		t.Fatalf("replaceReminderJobs (20h): %v", err)
	}
	if got := reminderHours(t, h, "b-2"); len(got) != 0 {
		t.Errorf("moved to 20h ahead: reminders = %v; want none (24h already due)", got)
	}
	if err := h.replaceReminderJobs(ctx, "b-2", "no-such-event-type", time.Now().UTC().Add(48*time.Hour)); err != nil {
		t.Fatalf("replaceReminderJobs (48h): %v", err)
	}
	if got := reminderHours(t, h, "b-2"); len(got) != 1 || got[0] != 24 {
		t.Errorf("moved to 48h ahead: reminders = %v; want [24]", got)
	}
}
