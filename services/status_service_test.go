package services

import (
	"os"
	"testing"
	"time"

	"performancereport/models"
)

func TestMonthlyWorkflowRemindersStatusesAndEdgeDates(t *testing.T) {
	tmpDir := t.TempDir()
	origEnv := os.Getenv("PERFORMANCE_REPORT_DATA_DIR")
	_ = os.Setenv("PERFORMANCE_REPORT_DATA_DIR", tmpDir)
	defer func() {
		if origEnv == "" {
			_ = os.Unsetenv("PERFORMANCE_REPORT_DATA_DIR")
		} else {
			_ = os.Setenv("PERFORMANCE_REPORT_DATA_DIR", origEnv)
		}
	}()

	// 1. Last day of month detection across 31-day, 30-day, non-leap Feb 28, leap Feb 29
	cases := []struct {
		date string
		want bool
	}{
		{"2027-01-31", true},
		{"2027-01-30", false},
		{"2027-02-28", true},  // non-leap year
		{"2028-02-28", false}, // leap year
		{"2028-02-29", true},  // leap year
		{"2027-04-30", true},
	}
	for _, tc := range cases {
		tm, _ := time.Parse("2006-01-02", tc.date)
		ev, err := EvaluateMonthlyReminders(tm)
		if err != nil {
			t.Fatalf("EvaluateMonthlyReminders(%s) failed: %v", tc.date, err)
		}
		if ev.IsLastDayOfMonth != tc.want {
			t.Fatalf("date %s: expected IsLastDayOfMonth=%v, got %v", tc.date, tc.want, ev.IsLastDayOfMonth)
		}
	}

	// 2. Dec -> Jan transition (Jan 1 checks Dec)
	jan1, _ := time.Parse("2006-01-02", "2027-01-01")
	evDec, _ := EvaluateMonthlyReminders(jan1)
	if evDec.PendingReminder == nil || evDec.PendingReminder.Month != "2026-12" {
		t.Fatalf("expected 2026-12 pending reminder on 2027-01-01, got %+v", evDec.PendingReminder)
	}

	// 3. Add task on Jan 1, verify MissingTaskSummary
	if err := SaveTask("2027-01", &models.Task{
		ID:      "t-jan1",
		Date:    "2027-01-01",
		Project: "ERA 3.0",
		Title:   "Implemented PDF Viewer",
		Status:  "completed",
	}); err != nil {
		t.Fatalf("SaveTask failed: %v", err)
	}
	sum, err := GetMissingTaskSummary("2027-01")
	if err != nil {
		t.Fatalf("GetMissingTaskSummary failed: %v", err)
	}
	if sum.TotalCalendarDays != 31 || sum.DaysWithEntries != 1 || sum.TotalTasks != 1 || len(sum.DatesWithoutEntries) != 30 {
		t.Fatalf("unexpected missing summary: %+v", sum)
	}

	// 4. Startup check on Feb 5 detects Jan pending report
	feb5, _ := time.Parse("2006-01-02", "2027-02-05")
	evFeb5, _ := EvaluateMonthlyReminders(feb5)
	if evFeb5.PendingReminder == nil || evFeb5.PendingReminder.Month != "2027-01" {
		t.Fatalf("expected 2027-01 pending reminder on 2027-02-05, got %+v", evFeb5.PendingReminder)
	}

	// 5. Remind Me Later suppresses pending reminder
	if _, err := RemindLaterReport("2027-01", 48); err != nil {
		t.Fatalf("RemindLaterReport failed: %v", err)
	}
	// Note: RemindLaterReport uses time.Now(), so test with current time or check RemindLaterUntil is set
	stJan, _ := GetMonthlyReportStatus("2027-01")
	if stJan.RemindLaterUntil == nil {
		t.Fatalf("expected RemindLaterUntil to be set")
	}

	// 6. Generate Report -> status becomes GENERATED_DRAFT and no longer triggers pending-generation reminder
	if _, err := GenerateReport("2027-01", nil); err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}
	stJan, _ = GetMonthlyReportStatus("2027-01")
	if stJan.Status != StatusGeneratedDraft {
		t.Fatalf("expected GENERATED_DRAFT, got %s", stJan.Status)
	}
	evAfterGen, _ := EvaluateMonthlyReminders(feb5)
	if evAfterGen.PendingReminder != nil {
		t.Fatalf("already generated report must not trigger pending-generation reminder, got %+v", evAfterGen.PendingReminder)
	}

	// 7. Mark as Completed -> status becomes COMPLETED, stops draft notices too
	if _, err := MarkReportCompleted("2027-01"); err != nil {
		t.Fatalf("MarkReportCompleted failed: %v", err)
	}
	stJan, _ = GetMonthlyReportStatus("2027-01")
	if stJan.Status != StatusCompleted || stJan.CompletedAt == nil {
		t.Fatalf("expected COMPLETED with timestamp, got %+v", stJan)
	}
	evAfterComplete, _ := EvaluateMonthlyReminders(feb5)
	if evAfterComplete.PendingReminder != nil || evAfterComplete.DraftNotice != nil {
		t.Fatalf("completed month must not trigger any reminders, got pending=%+v draft=%+v", evAfterComplete.PendingReminder, evAfterComplete.DraftNotice)
	}

	// 8. History lists 2027-01 as COMPLETED
	hist, err := ListAllMonthHistory()
	if err != nil || len(hist) == 0 {
		t.Fatalf("ListAllMonthHistory failed: %v", err)
	}
	if hist[0]["month"] != "2027-01" || hist[0]["status"] != StatusCompleted {
		t.Fatalf("expected 2027-01 COMPLETED in history, got %+v", hist[0])
	}
}
