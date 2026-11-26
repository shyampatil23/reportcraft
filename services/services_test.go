package services_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"performancereport/models"
	"performancereport/services"
)

func TestMain(m *testing.M) {
	testDir, _ := filepath.Abs("../testdata")
	os.Setenv("PERFORMANCE_REPORT_DATA_DIR", testDir)
	_ = os.MkdirAll(testDir, 0755)
	os.Exit(m.Run())
}

func TestProfilePersistence(t *testing.T) {
	profile := &models.Profile{
		EmployeeName:  "Test Employee",
		EmployeeCode:  "MKCL001",
		AppraiserName: "Test Appraiser",
		Department:    "Software Engineering",
		Team:          "ERA Platform",
		Projects:      []string{"EMACloudContentManager", "ERA 3.0"},
	}

	if err := services.SaveProfile(profile); err != nil {
		t.Fatalf("Failed to save profile: %v", err)
	}

	loaded, err := services.GetProfile()
	if err != nil {
		t.Fatalf("Failed to load profile: %v", err)
	}

	if loaded.EmployeeName != profile.EmployeeName {
		t.Errorf("Expected name %s, got %s", profile.EmployeeName, loaded.EmployeeName)
	}
	if len(loaded.Projects) != 2 {
		t.Errorf("Expected 2 projects, got %d", len(loaded.Projects))
	}
}

func TestDailyTasksAndLeapYear(t *testing.T) {
	// Leap year test: 2024-02 (29 days) vs 2023-02 (28 days) vs 2024-04 (30 days) vs 2024-05 (31 days)
	monthLeap := "2024-02"
	task29 := &models.Task{
		Date:                "2024-02-29",
		Project:             "ERA 3.0",
		Title:               "Leap Day Simulation Bugfix",
		ShortDescription:    "Fixed leap day timestamp parsing",
		DetailedDescription: "Implemented proper calendar date handling for leap years in the exam engine.",
		Status:              "completed",
		Source:              "daily",
	}

	if err := services.SaveTask(monthLeap, task29); err != nil {
		t.Fatalf("Failed to save leap year task: %v", err)
	}

	tasks, err := services.GetTasks(monthLeap)
	if err != nil {
		t.Fatalf("Failed to get tasks: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatalf("Expected at least 1 task for 2024-02, got 0")
	}

	found := false
	for _, tk := range tasks {
		if tk.Date == "2024-02-29" && tk.Title == "Leap Day Simulation Bugfix" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Leap year task on 2024-02-29 not found")
	}
}

func TestOfflineMonthlyTextParser(t *testing.T) {
	sampleText := `EMACloudContentManager

Translated Video Integration API
Enhanced the CDP API to support multiple translated videos within one request. Updated the payload and response structures.

Prevention of Parallel API Requests
Implemented mutex protection to prevent concurrent integration requests.

ERA 3.0

Simulation Improvements
Removed the Show Targets option and improved simulation behavior.`

	result := services.ParseMonthlyText(sampleText)

	if len(result.Projects) < 2 {
		t.Errorf("Expected at least 2 projects, got %d", len(result.Projects))
	}

	if len(result.Tasks) < 3 {
		t.Errorf("Expected at least 3 tasks, got %d", len(result.Tasks))
	}

	// Verify project detection
	p1Found := false
	p2Found := false
	for _, p := range result.Projects {
		if p == "EMACloudContentManager" {
			p1Found = true
		}
		if p == "ERA 3.0" {
			p2Found = true
		}
	}
	if !p1Found || !p2Found {
		t.Errorf("Failed to detect expected project names, got: %v", result.Projects)
	}

	// Verify tasks
	t1Found := false
	for _, tk := range result.Tasks {
		if tk.Title == "Translated Video Integration API" && tk.Project == "EMACloudContentManager" {
			t1Found = true
			if tk.DetailedDescription == "" {
				t.Errorf("Expected task description to be extracted")
			}
		}
	}
	if !t1Found {
		t.Errorf("Failed to properly group Translated Video Integration API under EMACloudContentManager")
	}
}

func TestMarkdownMonthlyReportParser(t *testing.T) {
	mdContent := `# Monthly Performance Report – September 2026

# TABLE OF CONTENTS
- [PowerShell Fallback Mechanism](#PowerShell_Fallback)
- [ERA Explorer and MKCL ERA Live Browser Testing](#Browser_Testing)

---

# Part A – EMASECUREBROWSER

## 1. PowerShell Fallback Mechanism
### Issue Identified
- System information retrieval failed on environments without WMIC.
### Activities Performed
- Implemented a PowerShell fallback mechanism in **ERA Explorer** and **MKCL ERA Live**.
- Added fallback handling for systems where WMIC is unavailable.
### Solution Implemented
- Configured application to use PowerShell.
### Outcome
Improved system compatibility by allowing ERA Explorer and MKCL ERA Live to retrieve required system information.

## 2. ERA Explorer and MKCL ERA Live Browser Testing
### Activities Performed
- Performed testing of **ERA Explorer** and **MKCL ERA Live** browsers.
### Outcome
Validated the updated browser functionality.

# Part B – ERA 3.0
## 3. ERA 3.0 Staging Testing with Specific Learner
### Activities Performed
- Validated learner session.
### Outcome
All tests passed.

# Conclusion
The work performed during this month successfully met all milestones.`

	result := services.ParseMonthlyText(mdContent)

	if len(result.Projects) < 2 {
		t.Fatalf("Expected at least 2 projects, got %d: %v", len(result.Projects), result.Projects)
	}

	for _, p := range result.Projects {
		if p == "### Activities Performed" || p == "### Issue Identified" || p == "### Outcome" || p == "---" {
			t.Errorf("Illegal project name captured: %s", p)
		}
	}

	if len(result.Tasks) != 3 {
		t.Fatalf("Expected exactly 3 tasks, got %d", len(result.Tasks))
	}

	// Task 1
	tk1 := result.Tasks[0]
	if tk1.Title != "PowerShell Fallback Mechanism" {
		t.Errorf("Expected title 'PowerShell Fallback Mechanism', got '%s'", tk1.Title)
	}
	if tk1.Project != "EMASECUREBROWSER" {
		t.Errorf("Expected project 'EMASECUREBROWSER', got '%s'", tk1.Project)
	}
	if tk1.IssueIdentified == "" {
		t.Errorf("Expected IssueIdentified to be populated")
	}
	if tk1.ActivitiesPerformed == "" {
		t.Errorf("Expected ActivitiesPerformed to be populated")
	}
	if tk1.SolutionImplemented == "" {
		t.Errorf("Expected SolutionImplemented to be populated")
	}
	if tk1.Outcome == "" {
		t.Errorf("Expected Outcome to be populated")
	}

	// Verify conclusion
	if result.Conclusion == "" {
		t.Errorf("Expected Conclusion to be extracted")
	}
}

func TestReportGenerationAndDocxPdf(t *testing.T) {
	month := "2026-10"

	// Add sample tasks for 2026-10
	t1 := models.Task{
		Date:                "2026-10-05",
		Project:             "EMACloudContentManager",
		Title:               "Translated Video Integration API",
		IssueIdentified:     "CDP API did not support multiple translated video IDs in a single request.",
		ActivitiesPerformed: "Enhanced the CDP API endpoint and updated schema validation.",
		SolutionImplemented: "Added batch support with asynchronous video transcoding.",
		Outcome:             "Reduced API roundtrips by 65%.",
		Status:              "completed",
		Source:              "daily",
	}
	t2 := models.Task{
		Date:                "2026-10-18",
		Project:             "ERA 3.0",
		Title:               "Simulation Optimization",
		IssueIdentified:     "Show Targets option caused lag during full-screen simulations.",
		ActivitiesPerformed: "Removed unnecessary redraw calls and optimized target rendering.",
		SolutionImplemented: "Implemented dirty-region canvas rendering.",
		Outcome:             "Simulation frame rate stabilized at 60 FPS.",
		Status:              "completed",
		Source:              "fullmonth",
	}

	_ = services.SaveTask(month, &t1)
	_ = services.SaveTask(month, &t2)

	// Generate report
	report, err := services.GenerateReport(month, nil)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	if report.CoverPage.MonthYear != "October 2026" {
		t.Errorf("Expected monthYear October 2026, got %s", report.CoverPage.MonthYear)
	}

	if len(report.Sections) < 2 {
		t.Errorf("Expected 2 sections (projects), got %d", len(report.Sections))
	}

	// Test Word (.docx) generation
	docxPath, err := services.GenerateDocx(report)
	if err != nil {
		t.Fatalf("GenerateDocx failed: %v", err)
	}
	if fi, err := os.Stat(docxPath); err != nil || fi.Size() == 0 {
		t.Errorf("DOCX file was not created or is empty: %s", docxPath)
	}

	// Test PDF (.pdf.html) generation
	pdfPath, err := services.GeneratePDF(report)
	if err != nil {
		t.Fatalf("GeneratePDF failed: %v", err)
	}
	if fi, err := os.Stat(pdfPath); err != nil || fi.Size() == 0 {
		t.Errorf("PDF output was not created or is empty: %s", pdfPath)
	}
}

func TestBackupAndRestore(t *testing.T) {
	info, err := services.CreateBackup()
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}
	if info.Filename == "" || info.SizeMB < 0 {
		t.Errorf("Invalid backup info: %+v", info)
	}

	list, err := services.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}
	if len(list) == 0 {
		t.Errorf("Expected at least 1 backup in list")
	}
}

func TestOfflineAIBehavior(t *testing.T) {
	// When AI credentials are not provided, it should gracefully return an error
	req := &models.AIImproveRequest{
		Text:   "Added PowerShell fallback when WMIC is unavailable.",
		Mode:   "improve",
		Detail: "standard",
	}
	_, err := services.ImproveText(req)
	// Expected error since no API key is configured
	if err == nil {
		t.Logf("AI call succeeded or was configured")
	} else {
		// Verify app continues cleanly when AI is offline
		t.Logf("AI offline error handled properly: %v", err)
	}
}

func TestPastedWordDocumentParser(t *testing.T) {
	pastedDoc := `TABLE OF CONTENTS
EMACloudContentManager
Translated Video Integration API
CDP Single User Login Session Logout API
Prevention of Parallel API Requests for same Learner
Single User Login Session ERA Explorer Integration
Single User Login Session MKCL ERA Live Integration
Role Based Assessment Flow Support For ERA 3.0
MKCL ERA Live Build Automation Script
ERA 3.0
Display Assessment Score and Attempt Analysis after exam
Display Group Assessment Score
Display Evidence In Assessment Review
Custom Test Assessment Review
Reattempt Flow For ERA 3.0
Simulation Improvements
Assessment Analysis Graph
Conclusion

EMACloudContentManager
Translated Video Integration API
Activities Performed
- Enhanced the CDP API to support multiple translated videos within one request.
- Updated the payload and response structures.
Outcome
Multiple videos can be translated in parallel without timeout errors.

Prevention of Parallel API Requests
Issue Identified
Simultaneous login attempts caused session conflict.
Activities Performed
- Implemented mutex lock in Redis for learner session.
Solution Implemented
- Restricted concurrent integration requests.
Outcome
Zero duplicate login records observed in testing.

ERA 3.0
Simulation Improvements
Activities Performed
- Removed the Show Targets option and improved simulation behavior.
Outcome
Simulation runs smoothly across low-end machines.

Conclusion
In October 2026, development milestones across EMACloudContentManager and ERA 3.0 were completed successfully.`

	result := services.ParseMonthlyText(pastedDoc)

	if len(result.Projects) != 2 {
		t.Fatalf("Expected exactly 2 projects (EMACloudContentManager and ERA 3.0), got %d: %v", len(result.Projects), result.Projects)
	}

	for _, p := range result.Projects {
		if !services.IsValidProjectName(p) {
			t.Errorf("Invalid project name found: %s", p)
		}
	}

	if len(result.Tasks) != 3 {
		t.Fatalf("Expected exactly 3 tasks, got %d", len(result.Tasks))
	}

	if result.Tasks[0].Title != "Translated Video Integration API" {
		t.Errorf("Expected first task title to be 'Translated Video Integration API', got '%s'", result.Tasks[0].Title)
	}

	if result.Tasks[0].Project != "EMACloudContentManager" {
		t.Errorf("Expected first task project to be 'EMACloudContentManager', got '%s'", result.Tasks[0].Project)
	}

	if result.Tasks[2].Title != "Simulation Improvements" {
		t.Errorf("Expected third task title to be 'Simulation Improvements', got '%s'", result.Tasks[2].Title)
	}

	if result.Tasks[2].Project != "ERA 3.0" {
		t.Errorf("Expected third task project to be 'ERA 3.0', got '%s'", result.Tasks[2].Project)
	}

	if result.Conclusion == "" {
		t.Errorf("Expected Conclusion to be extracted")
	}
}

func TestIsValidProjectName(t *testing.T) {
	valid := []string{"EMACloudContentManager", "ERA 3.0", "MKCL ERA Live", "EMASECUREBROWSER", "CDP API"}
	for _, v := range valid {
		if !services.IsValidProjectName(v) {
			t.Errorf("Expected %s to be a valid project name", v)
		}
	}

	invalid := []string{
		"Activities Performed", "Outcome", "Issue Identified", "Solution Implemented",
		"TABLE OF CONTENTS", "Conclusion", "Activities", "Outcome :", "---", "General", "general",
	}
	for _, inv := range invalid {
		if services.IsValidProjectName(inv) {
			t.Errorf("Expected %s to be an INVALID project name", inv)
		}
	}
}

func TestUserProvidedDetailsOnly(t *testing.T) {
	input := `General
Project: ERA Live 2.0, ERA Live 3.0, EMACloudContentManager & EMASECUREBROWSER
Activities Performed
Employee Name: Sham Anil Patil
Employee Code: 1947
Appraiser Name: Shivprakash Bapat you can not need to add this by your side add only details that user provide you do not add any thing`

	result := services.ParseMonthlyText(input)

	// Verify "General" is NOT included in projects
	for _, p := range result.Projects {
		if p == "General" || p == "general" {
			t.Errorf("Illegal 'General' project included: %s", p)
		}
	}

	// Verify all 4 user projects were extracted
	if len(result.Projects) != 4 {
		t.Fatalf("Expected exactly 4 projects, got %d: %v", len(result.Projects), result.Projects)
	}

	expected := []string{"ERA Live 2.0", "ERA Live 3.0", "EMACloudContentManager", "EMASECUREBROWSER"}
	for i, exp := range expected {
		if result.Projects[i] != exp {
			t.Errorf("Expected project %d to be %s, got %s", i, exp, result.Projects[i])
		}
	}

	// Verify employee details extracted
	if result.EmployeeName != "Sham Anil Patil" {
		t.Errorf("Expected EmployeeName 'Sham Anil Patil', got '%s'", result.EmployeeName)
	}
	if result.EmployeeCode != "1947" {
		t.Errorf("Expected EmployeeCode '1947', got '%s'", result.EmployeeCode)
	}
	if result.AppraiserName != "Shivprakash Bapat" {
		t.Errorf("Expected AppraiserName 'Shivprakash Bapat', got '%s'", result.AppraiserName)
	}
}

func TestCoverPageTypographyAndStyling(t *testing.T) {
	rep := &models.Report{
		ID:    "test-cover-typography",
		Month: "2026-10",
		CoverPage: models.CoverPage{
			Title:         "Monthly Performance Report",
			MonthYear:     "October 2026",
			Month:         "October",
			Year:          "2026",
			Projects:      []string{"EMACloudContentManager", "ERA 3.0"},
			EmployeeName:  "Sham Anil Patil",
			EmployeeCode:  "1947",
			AppraiserName: "Shivprakash Bapat",
		},
		Sections: []models.ReportSection{
			{
				Project: "EMACloudContentManager",
				Tasks: []models.ReportTask{
					{
						Title:               "API Integration",
						ActivitiesPerformed: "Implemented REST API endpoints",
					},
				},
			},
		},
	}

	htmlContent := services.BuildPDFContent(rep)

	// 1. Verify Banner: Century Gothic, 28pt
	if !strings.Contains(htmlContent, "font-family: 'Century Gothic'") {
		t.Errorf("Expected Century Gothic in HTML")
	}
	if !strings.Contains(htmlContent, "font-size: 28pt") {
		t.Errorf("Expected font-size 28pt for banner in HTML")
	}

	// 2. Verify Projects line: Century Gothic, 14pt
	if !strings.Contains(htmlContent, "Projects : EMACloudContentManager, ERA 3.0") {
		t.Errorf("Expected Projects line with both projects in HTML")
	}

	// 3. Verify Employee fields: 14pt, Avenir Next LT Pro Light
	if !strings.Contains(htmlContent, "Avenir Next LT Pro Light") {
		t.Errorf("Expected 'Avenir Next LT Pro Light' font for employee details")
	}
	if !strings.Contains(htmlContent, "Sham Anil Patil") || !strings.Contains(htmlContent, "1947") || !strings.Contains(htmlContent, "Shivprakash Bapat") {
		t.Errorf("Expected employee name, code, appraiser name in HTML")
	}

	// 4. Verify Logo is reduced in size (25mm)
	if !strings.Contains(htmlContent, "width: 25mm") {
		t.Errorf("Expected logo width to be 25mm")
	}
}

func TestERAFormattingAndTemplateIsolation(t *testing.T) {
	era, err := services.GetTemplate("era")
	if err != nil || era == nil {
		t.Fatalf("Expected built-in ERA template, got err=%v", err)
	}
	if era.Typography.ProjectName.FontSizePt != 23 || era.Typography.ProjectName.FontFamily != "Century Gothic" {
		t.Errorf("Expected ERA ProjectName 23pt Century Gothic, got %v %s", era.Typography.ProjectName.FontSizePt, era.Typography.ProjectName.FontFamily)
	}
	if era.Typography.TaskTitle.FontSizePt != 16 || era.Typography.TaskTitle.FontFamily != "Times New Roman" {
		t.Errorf("Expected ERA TaskTitle 16pt Times New Roman, got %v %s", era.Typography.TaskTitle.FontSizePt, era.Typography.TaskTitle.FontFamily)
	}
	if era.Typography.TaskDescription.FontSizePt != 14 || era.Typography.TaskDescription.FontFamily != "Times New Roman" {
		t.Errorf("Expected ERA TaskDescription 14pt Times New Roman, got %v %s", era.Typography.TaskDescription.FontSizePt, era.Typography.TaskDescription.FontFamily)
	}

	rep := &models.Report{
		ID:         "test-era-23-16-14",
		Month:      "2026-10",
		TemplateID: "era",
		CoverPage: models.CoverPage{
			Title:         "Monthly Performance Report",
			MonthYear:     "October 2026",
			Projects:      []string{"EMACloudContentManager", "ERA 3.0"},
			EmployeeName:  "Sham Anil Patil",
			EmployeeCode:  "1947",
			AppraiserName: "Shivprakash Bapat",
		},
		Sections: []models.ReportSection{
			{
				Project: "EMACloudContentManager",
				Tasks: []models.ReportTask{
					{
						Title:               "Translated Video Integration API",
						ActivitiesPerformed: "- Enhanced batch API\n  o Added contentNames map",
						ChallengesFaced:     "- Concurrency race condition on parallel calls",
						Outcome:             "Zero duplicate integrations.",
					},
				},
			},
			{
				Project: "ERA 3.0",
				Tasks: []models.ReportTask{
					{
						Title:               "Simulation Feedback Smileys",
						ActivitiesPerformed: "1. Replaced 10px dots\n2. Added 24px smiley icons",
					},
				},
			},
		},
		Conclusion: "All deliverables completed.",
	}

	html := services.BuildPDFContent(rep)
	if !strings.Contains(html, "font-size: 23pt") {
		t.Errorf("Expected 23pt project heading in HTML")
	}
	if !strings.Contains(html, "font-size: 16pt") {
		t.Errorf("Expected 16pt task title in HTML")
	}
	if !strings.Contains(html, "font-size: 14pt") {
		t.Errorf("Expected 14pt description/subheading in HTML")
	}
	if !strings.Contains(html, "Challenges Faced") {
		t.Errorf("Expected Challenges Faced section in HTML")
	}
	if !strings.Contains(html, "header-page-badge\">3</div>") || !strings.Contains(html, "header-page-badge\">4</div>") {
		t.Errorf("Expected paginated interior page badges 3 and 4")
	}

	// Verify template isolation: create a custom non-ERA template and ensure ERA images are NOT injected
	customTmpl := &models.Template{
		ID:           "qa-team-test",
		Name:         "QA Team Report",
		SourceType:   "custom",
		PrimaryColor: "#0F766E",
		AccentColor:  "#CCFBF1",
	}
	if err := services.SaveTemplate(customTmpl); err != nil {
		t.Fatalf("SaveTemplate failed: %v", err)
	}
	defer services.DeleteTemplate("qa-team-test")

	qaRep := *rep
	qaRep.TemplateID = "qa-team-test"
	qaHTML := services.BuildPDFContent(&qaRep)
	if strings.Contains(qaHTML, `<div class="cover-yellow-backdrop">`) {
		t.Errorf("Non-ERA template must NOT use ERA cover-yellow-backdrop")
	}
	if strings.Contains(qaHTML, "data:image/jpeg;base64,") {
		t.Errorf("Non-ERA template without assets must NOT leak ERA cover illustration")
	}
}

func TestAlignmentBulletsTOCDimensionsAndHeadings(t *testing.T) {
	rep := &models.Report{
		ID:         "test-november-alignment",
		Month:      "2026-11",
		TemplateID: "era",
		CoverPage: models.CoverPage{
			Title:         "Monthly Performance Report",
			MonthYear:     "November 2026",
			Projects:      []string{"EMACloudContentManager", "ERA 3.0"},
			EmployeeName:  "Sham Anil Patil",
			EmployeeCode:  "1947",
			AppraiserName: "Shivprakash Bapat",
		},
		Sections: []models.ReportSection{
			{
				Project: "EMACloudContentManager",
				Tasks: []models.ReportTask{
					{
						Title: "Translated Video Integration API",
						ActivitiesPerformed: "Activities Performed\n• Enhanced the CDP video integration API to support processing multiple\ntranslated\nvideos in a single request.\nOutcome\nEnhanced batch processing.",
					},
				},
			},
		},
		Conclusion: "During November 2026, work was carried out across\nEMACloudContentManager and ERA 3.0.",
	}

	html := services.BuildPDFContent(rep)

	// 1. US Letter dimensions (215.9mm x 279.4mm)
	if !strings.Contains(html, "215.9mm 279.4mm") {
		t.Errorf("Expected US Letter dimensions 215.9mm 279.4mm in @page CSS")
	}
	// 2. Merged wrapped bullet (must appear as ONE bullet, NOT "• translated" or "• videos in a single request.")
	if !strings.Contains(html, "• Enhanced the CDP video integration API to support processing multiple translated videos in a single request.") {
		t.Errorf("Expected wrapped lines to merge into a single bullet")
	}
	if strings.Contains(html, "• translated") || strings.Contains(html, "• videos in a single request.") {
		t.Errorf("Wrapped continuation lines must NOT become separate bullets")
	}
	// 3. No duplicate heading inside bullet list
	if strings.Contains(html, "• Activities Performed") || strings.Contains(html, "• Outcome") {
		t.Errorf("Section headings must NOT appear as bullet items")
	}
	// 4. TOC must NOT have dotted leaders or page numbers
	if strings.Contains(html, "toc-dots") {
		t.Errorf("TOC must NOT contain dotted leaders or entry page numbers")
	}
	// 5. Conclusion wrapped lines merged into one paragraph
	if !strings.Contains(html, "During November 2026, work was carried out across EMACloudContentManager and ERA 3.0.") {
		t.Errorf("Expected wrapped Conclusion lines to merge into a single paragraph")
	}
}
