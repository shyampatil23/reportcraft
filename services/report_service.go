package services

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"performancereport/models"
	"performancereport/storage"
)

func GetReport(month string) (*models.Report, error) {
	var r models.Report
	if err := storage.ReadJSON(storage.ReportDraftPath(month), &r); err != nil {
		return nil, err
	}
	if r.ID == "" {
		return nil, nil // no report yet
	}
	return &r, nil
}

func SaveReport(r *models.Report) error {
	r.UpdatedAt = time.Now().Format(time.RFC3339)
	return storage.WriteJSON(storage.ReportDraftPath(r.Month), r)
}

// GenerateReport builds a report from tasks for the given month using the default template.
func GenerateReport(month string, selectedTaskIDs []string) (*models.Report, error) {
	return GenerateReportWithOptions(month, selectedTaskIDs, "", "")
}

// GenerateReportWithOptions builds a report from tasks for the given month with explicit template and formatting mode.
func GenerateReportWithOptions(month string, selectedTaskIDs []string, templateID string, formattingMode string) (*models.Report, error) {
	profile, err := GetProfile()
	if err != nil {
		return nil, fmt.Errorf("profile error: %w", err)
	}

	tasks, err := GetTasks(month)
	if err != nil {
		return nil, fmt.Errorf("tasks error: %w", err)
	}

	// If selectedTaskIDs provided, filter
	if len(selectedTaskIDs) > 0 {
		sel := make(map[string]bool)
		for _, id := range selectedTaskIDs {
			sel[id] = true
		}
		filtered := tasks[:0]
		for _, t := range tasks {
			if sel[t.ID] {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}

	// Group by project, maintaining order of first appearance
	type projectGroup struct {
		project string
		tasks   []models.Task
	}
	projectOrder := []string{}
	projectMap := map[string][]models.Task{}
	for _, t := range tasks {
		projName := cleanProjectName(t.Project)
		if !IsValidProjectName(projName) || strings.EqualFold(projName, "General") {
			if len(profile.Projects) > 0 {
				projName = profile.Projects[0]
			} else {
				projName = ""
			}
		}
		if projName != "" {
			if _, exists := projectMap[projName]; !exists {
				projectOrder = append(projectOrder, projName)
			}
			projectMap[projName] = append(projectMap[projName], t)
		} else {
			if _, exists := projectMap[""]; !exists {
				projectOrder = append(projectOrder, "")
			}
			projectMap[""] = append(projectMap[""], t)
		}
	}

	sections := make([]models.ReportSection, 0, len(projectOrder))
	for i, proj := range projectOrder {
		pts := projectMap[proj]
		reportTasks := make([]models.ReportTask, 0, len(pts))
		for j, t := range pts {
			rt := models.ReportTask{
				TaskID:              t.ID,
				Title:               cleanDisplayTitle(t.Title),
				IssueIdentified:     t.IssueIdentified,
				ActivitiesPerformed: t.ActivitiesPerformed,
				SolutionImplemented: t.SolutionImplemented,
				Outcome:             t.Outcome,
				Order:               j + 1,
			}
			if rt.ActivitiesPerformed == "" {
				rt.ActivitiesPerformed = t.DetailedDescription
			}
			reportTasks = append(reportTasks, rt)
		}
		sections = append(sections, models.ReportSection{
			Project: proj,
			Tasks:   reportTasks,
			Order:   i + 1,
		})
	}

	// Parse month
	t, _ := time.Parse("2006-01", month)
	monthYear := t.Format("January 2006")

	// Build project list for cover: only genuine projects, deduplicated, no "General"
	projectNames := []string{}
	seenProj := make(map[string]bool)
	for _, p := range projectOrder {
		if p != "" && IsValidProjectName(p) && !strings.EqualFold(p, "General") && !seenProj[strings.ToLower(p)] {
			projectNames = append(projectNames, p)
			seenProj[strings.ToLower(p)] = true
		}
	}
	if len(projectNames) == 0 && profile != nil {
		for _, p := range profile.Projects {
			if p != "" && IsValidProjectName(p) && !strings.EqualFold(p, "General") && !seenProj[strings.ToLower(p)] {
				projectNames = append(projectNames, p)
				seenProj[strings.ToLower(p)] = true
			}
		}
	}

	// Check if user provided conclusion in full-month draft or existing report; never fabricate!
	var conclusionText string
	if draft, err := GetFullMonthDraft(month); err == nil && draft != nil && draft.RawText != "" {
		parsed := ParseMonthlyText(draft.RawText)
		if parsed != nil && parsed.Conclusion != "" {
			conclusionText = parsed.Conclusion
		}
	}
	if conclusionText == "" {
		if existing, err := GetReport(month); err == nil && existing != nil && existing.Conclusion != "" {
			conclusionText = existing.Conclusion
		}
	}

	if templateID == "" {
		if def, err := GetTemplate(""); err == nil && def != nil {
			templateID = def.ID
		} else {
			templateID = "era"
		}
	}
	if formattingMode == "" {
		formattingMode = "match_template"
	}

	now := time.Now()
	report := &models.Report{
		ID:             uuid.New().String(),
		Month:          month,
		TemplateID:     templateID,
		FormattingMode: formattingMode,
		SchemaVersion:  2,
		CoverPage: models.CoverPage{
			Title:         "Monthly Performance Report",
			Month:         t.Format("January"),
			Year:          t.Format("2006"),
			MonthYear:     monthYear,
			EmployeeName:  profile.EmployeeName,
			EmployeeCode:  profile.EmployeeCode,
			AppraiserName: profile.AppraiserName,
			Department:    profile.Department,
			Projects:      projectNames,
		},
		Sections:    sections,
		Conclusion:  conclusionText,
		Version:     1,
		Status:      "draft",
		GeneratedAt: now.Format(time.RFC3339),
		UpdatedAt:   now.Format(time.RFC3339),
	}

	if err := SaveReport(report); err != nil {
		return nil, err
	}
	_, _ = MarkReportGenerated(month, "", "")
	return report, nil
}

// GenerateConclusion creates a default conclusion paragraph from tasks.
func GenerateConclusion(profile *models.Profile, tasks []models.Task) string {
	if len(tasks) == 0 {
		return "No tasks were recorded for this reporting period."
	}
	projectSet := map[string]bool{}
	for _, t := range tasks {
		projectSet[t.Project] = true
	}
	projects := []string{}
	for p := range projectSet {
		if p != "" {
			projects = append(projects, p)
		}
	}
	sort.Strings(projects)

	name := profile.EmployeeName
	if name == "" {
		name = "The employee"
	}
	total := len(tasks)
	proj := strings.Join(projects, ", ")
	return fmt.Sprintf(
		"%s successfully completed %d tasks during this reporting period across the following projects: %s. "+
			"All deliverables were completed as per the project requirements. "+
			"The work carried out reflects the commitment to quality and timely delivery.",
		name, total, proj,
	)
}

// ListReports returns all reports, most recent first.
func ListReports() ([]*models.Report, error) {
	reportsDir := filepath.Join(storage.DataPath(), "reports")
	var reports []*models.Report

	err := filepath.WalkDir(reportsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == "draft.json" {
			var r models.Report
			if e := storage.ReadJSON(path, &r); e == nil && r.ID != "" {
				reports = append(reports, &r)
			}
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Month > reports[j].Month
	})
	return reports, nil
}

// SaveReportVersion saves a snapshot of the current report as a version.
func SaveReportVersion(r *models.Report) error {
	r.Version++
	versionPath := storage.ReportVersionPath(r.Month, r.Version)
	if err := storage.WriteJSON(versionPath, r); err != nil {
		return err
	}
	return SaveReport(r)
}

// GetFullMonthDraft gets the full-month draft for a month.
func GetFullMonthDraft(month string) (*models.FullMonthDraft, error) {
	var d models.FullMonthDraft
	if err := storage.ReadJSON(storage.FullMonthDraftPath(month), &d); err != nil {
		return nil, err
	}
	if d.Month == "" {
		d.Month = month
	}
	return &d, nil
}

// SaveFullMonthDraft saves the full-month draft.
func SaveFullMonthDraft(draft *models.FullMonthDraft) error {
	draft.SavedAt = time.Now().Format(time.RFC3339)
	return storage.WriteJSON(storage.FullMonthDraftPath(draft.Month), draft)
}
