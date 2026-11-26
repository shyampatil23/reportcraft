package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"performancereport/models"
	"performancereport/storage"
)

const (
	StatusInProgress     = "IN_PROGRESS"
	StatusGeneratedDraft = "GENERATED_DRAFT"
	StatusEditedDraft    = "EDITED_DRAFT"
	StatusCompleted      = "COMPLETED"
)

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// GetMonthlyReportStatus reads status.json for a month, or infers initial state from existing local JSON/files.
func GetMonthlyReportStatus(month string) (models.MonthlyReportStatus, error) {
	st := models.MonthlyReportStatus{
		Month:  month,
		Status: StatusInProgress,
	}
	path := storage.ReportStatusPath(month)
	if storage.FileExists(path) {
		if err := storage.ReadJSON(path, &st); err != nil {
			return st, err
		}
		if st.Month == "" {
			st.Month = month
		}
		if st.Status == "" {
			st.Status = StatusInProgress
		}
		return st, nil
	}

	// Infer from existing files if status.json hasn't been created yet
	if storage.FileExists(storage.ReportEditedDocxPath(month)) || storage.FileExists(storage.ReportEditorSavedPath(month)) {
		st.Status = StatusEditedDraft
		now := time.Now().Format(time.RFC3339)
		st.EditedAt = &now
		p := storage.ReportEditedPDFPath(month)
		d := storage.ReportEditedDocxPath(month)
		st.PDFPath = &p
		st.DOCXPath = &d
	} else if r, err := GetReport(month); err == nil && r != nil {
		st.Status = StatusGeneratedDraft
		ts := r.GeneratedAt
		if ts == "" {
			ts = time.Now().Format(time.RFC3339)
		}
		st.GeneratedAt = &ts
	}
	return st, nil
}

func saveMonthlyReportStatus(st models.MonthlyReportStatus) error {
	return storage.WriteJSON(storage.ReportStatusPath(st.Month), st)
}

func MarkReportGenerated(month, pdfPath, docxPath string) (models.MonthlyReportStatus, error) {
	st, _ := GetMonthlyReportStatus(month)
	now := time.Now().Format(time.RFC3339)
	st.Month = month
	st.Status = StatusGeneratedDraft
	st.GeneratedAt = &now
	st.RemindLaterUntil = nil
	if pdfPath != "" {
		st.PDFPath = &pdfPath
	}
	if docxPath != "" {
		st.DOCXPath = &docxPath
	}
	return st, saveMonthlyReportStatus(st)
}

func MarkReportEdited(month, pdfPath, docxPath string) (models.MonthlyReportStatus, error) {
	st, _ := GetMonthlyReportStatus(month)
	now := time.Now().Format(time.RFC3339)
	st.Month = month
	if st.Status != StatusCompleted {
		st.Status = StatusEditedDraft
	}
	st.EditedAt = &now
	if pdfPath != "" {
		st.PDFPath = &pdfPath
	}
	if docxPath != "" {
		st.DOCXPath = &docxPath
	}
	return st, saveMonthlyReportStatus(st)
}

// MarkReportCompleted finalizes the report, ensuring latest DOCX and PDF are preserved, and stops pending reminders.
func MarkReportCompleted(month string) (models.MonthlyReportStatus, error) {
	report, err := GetReport(month)
	if err != nil || report == nil {
		return models.MonthlyReportStatus{}, fmt.Errorf("report not generated yet for %s", month)
	}
	st, _ := GetMonthlyReportStatus(month)

	// Ensure latest PDF & DOCX exist on disk
	docxPath := storage.ReportEditedDocxPath(month)
	if !storage.FileExists(docxPath) {
		docxPath, err = GenerateDocx(report)
		if err != nil {
			return st, err
		}
	}
	pdfPath := storage.ReportEditedPDFPath(month)
	if !storage.FileExists(pdfPath) {
		pdfPath, err = GeneratePDF(report)
		if err != nil {
			return st, err
		}
	}

	now := time.Now().Format(time.RFC3339)
	st.Month = month
	st.Status = StatusCompleted
	st.CompletedAt = &now
	st.RemindLaterUntil = nil
	st.PDFPath = &pdfPath
	st.DOCXPath = &docxPath
	if st.GeneratedAt == nil {
		ts := report.GeneratedAt
		if ts == "" {
			ts = now
		}
		st.GeneratedAt = &ts
	}
	return st, saveMonthlyReportStatus(st)
}

func RemindLaterReport(month string, hours int) (models.MonthlyReportStatus, error) {
	if hours <= 0 {
		hours = 24
	}
	st, _ := GetMonthlyReportStatus(month)
	now := time.Now()
	nowStr := now.Format(time.RFC3339)
	untilStr := now.Add(time.Duration(hours) * time.Hour).Format(time.RFC3339)
	st.Month = month
	st.LastReminderAt = &nowStr
	st.RemindLaterUntil = &untilStr
	return st, saveMonthlyReportStatus(st)
}

// EvaluateMonthlyReminders checks whether today is the last day of the month and whether any previous
// month has a pending (never-generated) report that is not currently snoozed.
func EvaluateMonthlyReminders(now time.Time) (models.ReminderEvaluation, error) {
	curMonth := now.Format("2006-01")
	curSt, _ := GetMonthlyReportStatus(curMonth)
	lastDay := daysInMonth(now.Year(), now.Month())

	eval := models.ReminderEvaluation{
		CurrentDate:         now.Format("2006-01-02"),
		CurrentMonth:        curMonth,
		CurrentMonthName:    now.Format("January"),
		IsLastDayOfMonth:    now.Day() == lastDay,
		CurrentMonthStatus:  curSt,
		TasksChangedWarning: CheckTasksModifiedAfterGeneration(curMonth),
	}

	// Check previous month first (handles Dec -> Jan year transition naturally)
	prevTime := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -1, 0)
	prevMonth := prevTime.Format("2006-01")
	prevSt, _ := GetMonthlyReportStatus(prevMonth)

	isSnoozed := func(st models.MonthlyReportStatus) bool {
		if st.RemindLaterUntil == nil || *st.RemindLaterUntil == "" {
			return false
		}
		if until, err := time.Parse(time.RFC3339, *st.RemindLaterUntil); err == nil {
			return now.Before(until)
		}
		return false
	}

	// Only trigger pending-generation reminder if status is IN_PROGRESS (never generated) and not snoozed
	if prevSt.Status == StatusInProgress && !isSnoozed(prevSt) {
		tasks, _ := GetTasks(prevMonth)
		eval.PendingReminder = &models.PendingMonthReminder{
			Month:      prevMonth,
			MonthName:  prevTime.Format("January"),
			MonthLabel: prevTime.Format("January 2006"),
			Status:     prevSt.Status,
			TaskCount:  len(tasks),
		}
	} else if (prevSt.Status == StatusGeneratedDraft || prevSt.Status == StatusEditedDraft) && !isSnoozed(prevSt) {
		// Non-intrusive notice for unfinished draft (never confused with ungenerated report)
		tasks, _ := GetTasks(prevMonth)
		eval.DraftNotice = &models.PendingMonthReminder{
			Month:      prevMonth,
			MonthName:  prevTime.Format("January"),
			MonthLabel: prevTime.Format("January 2006"),
			Status:     prevSt.Status,
			TaskCount:  len(tasks),
		}
	}

	return eval, nil
}

// GetMissingTaskSummary calculates calendar days, days with entries, total tasks, and dates without entries.
func GetMissingTaskSummary(month string) (models.MissingTaskSummary, error) {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return models.MissingTaskSummary{}, fmt.Errorf("invalid month %s: %w", month, err)
	}
	totalDays := daysInMonth(t.Year(), t.Month())
	tasks, err := GetTasks(month)
	if err != nil {
		return models.MissingTaskSummary{}, err
	}

	daysMap := map[string]bool{}
	for _, task := range tasks {
		if task.Date != "" {
			daysMap[task.Date] = true
		}
	}

	var missing []string
	for d := 1; d <= totalDays; d++ {
		ds := fmt.Sprintf("%s-%02d", month, d)
		if !daysMap[ds] {
			missing = append(missing, ds)
		}
	}

	return models.MissingTaskSummary{
		Month:               month,
		MonthLabel:          t.Format("January 2006"),
		TotalCalendarDays:   totalDays,
		DaysWithEntries:     len(daysMap),
		TotalTasks:          len(tasks),
		DatesWithoutEntries: missing,
	}, nil
}

// CheckTasksModifiedAfterGeneration returns true if any task in the month was updated after the report was generated.
func CheckTasksModifiedAfterGeneration(month string) bool {
	st, err := GetMonthlyReportStatus(month)
	if err != nil || st.Status == StatusInProgress || st.GeneratedAt == nil {
		return false
	}
	genTime, err := time.Parse(time.RFC3339, *st.GeneratedAt)
	if err != nil {
		return false
	}
	tasks, err := GetTasks(month)
	if err != nil {
		return false
	}
	for _, task := range tasks {
		if !task.UpdatedAt.IsZero() && task.UpdatedAt.After(genTime) {
			return true
		}
	}
	return false
}

// ListAllMonthHistory returns status + report metadata for every month that has tasks or reports, sorted newest first.
func ListAllMonthHistory() ([]map[string]interface{}, error) {
	monthSet := map[string]bool{}

	if entries, err := os.ReadDir(filepath.Join(storage.DataPath(), "tasks")); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				m := strings.TrimSuffix(e.Name(), ".json")
				if len(m) == 7 {
					monthSet[m] = true
				}
			}
		}
	}
	if entries, err := os.ReadDir(filepath.Join(storage.DataPath(), "reports")); err == nil {
		for _, e := range entries {
			if e.IsDir() && len(e.Name()) == 7 {
				monthSet[e.Name()] = true
			}
		}
	}

	months := make([]string, 0, len(monthSet))
	for m := range monthSet {
		months = append(months, m)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(months)))

	out := make([]map[string]interface{}, 0, len(months))
	for _, m := range months {
		st, _ := GetMonthlyReportStatus(m)
		rep, _ := GetReport(m)
		tasks, _ := GetTasks(m)
		t, _ := time.Parse("2006-01", m)
		label := m
		if !t.IsZero() {
			label = t.Format("January 2006")
		}
		item := map[string]interface{}{
			"month":       m,
			"monthLabel":  label,
			"status":      st.Status,
			"statusInfo":  st,
			"taskCount":   len(tasks),
			"hasReport":   rep != nil,
			"report":      rep,
			"hasEdits":    storage.FileExists(storage.ReportEditedDocxPath(m)),
		}
		out = append(out, item)
	}
	return out, nil
}
