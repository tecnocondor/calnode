package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/calnode/calnode/internal/handler"
)

// Hours scoped to one event type replace the host's weekly hours for that event only:
// a training offered 14:00-16:00 while the rest of the calendar stays 09:00-17:00.

func addRule(t *testing.T, h *handler.Handler, key, body string) {
	t.Helper()
	code, resp := createRule(t, h, key, body)
	if code != http.StatusCreated {
		t.Fatalf("create rule %s: %d — %v", body, code, resp)
	}
}

func slotHoursUTC(t *testing.T, h *handler.Handler, slug, day string) []int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/event-types/"+slug+"/slots?from="+day+"&to="+day+"&tz=UTC", nil)
	req.SetPathValue("slug", slug)
	rec := httptest.NewRecorder()
	h.GetSlots(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("slots %s: %d — %s", slug, rec.Code, rec.Body.String())
	}
	var resp struct {
		Slots []struct {
			Start string `json:"start"`
		} `json:"slots"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode slots: %v", err)
	}
	hours := make([]int, 0, len(resp.Slots))
	for _, s := range resp.Slots {
		ts, err := time.Parse(time.RFC3339, s.Start)
		if err != nil {
			t.Fatalf("parse %q: %v", s.Start, err)
		}
		hours = append(hours, ts.UTC().Hour())
	}
	return hours
}

func TestEventHours_replaceWeeklyHoursForThatEventOnly(t *testing.T) {
	h, key, _ := setupWorkspace(t)
	monday := nextMonday()

	// The host's weekly hours: Monday 09:00-17:00.
	addRule(t, h, key, `{"day_of_week":1,"start_time":"09:00","end_time":"17:00"}`)

	training := createEventType(t, h, key, `{"slug":"training","name":"Training","duration_minutes":60}`)
	other := createEventType(t, h, key, `{"slug":"intro","name":"Intro","duration_minutes":60}`)
	addRule(t, h, key, fmt.Sprintf(`{"event_type_id":%q,"day_of_week":1,"start_time":"14:00","end_time":"16:00"}`, training["id"]))

	got := slotHoursUTC(t, h, "training", monday)
	if len(got) != 2 || got[0] != 14 || got[1] != 15 {
		t.Errorf("training slots = %v; want only 14:00 and 15:00", got)
	}

	// The other event keeps the weekly hours.
	rest := slotHoursUTC(t, h, "intro", monday)
	if len(rest) == 0 || rest[0] != 9 || rest[len(rest)-1] != 16 {
		t.Errorf("intro slots = %v; want 09:00 through 16:00", rest)
	}
	_ = other
}

func TestEventHours_dayWithoutOwnHoursIsClosed(t *testing.T) {
	h, key, _ := setupWorkspace(t)
	tuesday := func() string {
		d, _ := time.Parse("2006-01-02", nextMonday())
		return d.AddDate(0, 0, 1).Format("2006-01-02")
	}()

	addRule(t, h, key, `{"day_of_week":2,"start_time":"09:00","end_time":"17:00"}`)
	training := createEventType(t, h, key, `{"slug":"training","name":"Training","duration_minutes":60}`)
	// Own hours only on Monday: Tuesday must not fall back to the weekly hours.
	addRule(t, h, key, fmt.Sprintf(`{"event_type_id":%q,"day_of_week":1,"start_time":"14:00","end_time":"16:00"}`, training["id"]))

	if got := slotHoursUTC(t, h, "training", tuesday); len(got) != 0 {
		t.Errorf("training on Tuesday = %v; want no slots", got)
	}
}

func TestListAvailabilityRules_withoutFilterReturnsOnlyWeeklyHours(t *testing.T) {
	h, key, _ := setupWorkspace(t)

	addRule(t, h, key, `{"day_of_week":1,"start_time":"09:00","end_time":"17:00"}`)
	training := createEventType(t, h, key, `{"slug":"training","name":"Training","duration_minutes":60}`)
	addRule(t, h, key, fmt.Sprintf(`{"event_type_id":%q,"day_of_week":1,"start_time":"14:00","end_time":"16:00"}`, training["id"]))

	list := func(path string) []map[string]any {
		req := authReq(http.MethodGet, path, "", key)
		rec := httptest.NewRecorder()
		h.RequireAuth(h.ListAvailabilityRules)(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("list %s: %d — %s", path, rec.Code, rec.Body.String())
		}
		var resp struct {
			Items []map[string]any `json:"items"`
		}
		json.Unmarshal(rec.Body.Bytes(), &resp)
		return resp.Items
	}

	weekly := list("/v1/availability-rules")
	if len(weekly) != 1 || weekly[0]["start_time"] != "09:00" || weekly[0]["event_type_id"] != nil {
		t.Errorf("weekly rules = %v; want only the global 09:00-17:00", weekly)
	}
	own := list(fmt.Sprintf("/v1/availability-rules?event_type_id=%s", training["id"]))
	if len(own) != 1 || own[0]["start_time"] != "14:00" {
		t.Errorf("training rules = %v; want only 14:00-16:00", own)
	}
}
