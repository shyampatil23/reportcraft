package models

import "time"

// Profile represents the employee's profile
type Profile struct {
	EmployeeName  string   `json:"employeeName"`
	EmployeeCode  string   `json:"employeeCode"`
	AppraiserName string   `json:"appraiserName"`
	Department    string   `json:"department"`
	Team          string   `json:"team"`
	Projects      []string `json:"projects"`
	UpdatedAt     string   `json:"updatedAt"`
}

// Task represents a single work task entry
type Task struct {
	ID                  string    `json:"id"`
	Date                string    `json:"date"`
	Project             string    `json:"project"`
	Title               string    `json:"title"`
	ShortDescription    string    `json:"shortDescription"`
	DetailedDescription string    `json:"detailedDescription"`
	IssueIdentified     string    `json:"issueIdentified"`
	ActivitiesPerformed string    `json:"activitiesPerformed"`
	SolutionImplemented string    `json:"solutionImplemented"`
	Outcome             string    `json:"outcome"`
	Status              string    `json:"status"` // pending, in-progress, completed
	Tags                []string  `json:"tags"`
	Source              string    `json:"source"` // daily, fullmonth
	OriginalTaskIDs     []string  `json:"originalTaskIds,omitempty"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// ParsedTask represents a task extracted from free-form text
type ParsedTask struct {
	ID                  string `json:"id"`
	Project             string `json:"project"`
	Title               string `json:"title"`
	ShortDescription    string `json:"shortDescription"`
	DetailedDescription string `json:"detailedDescription"`
	IssueIdentified     string `json:"issueIdentified"`
	ActivitiesPerformed string `json:"activitiesPerformed"`
	SolutionImplemented string `json:"solutionImplemented"`
	Outcome             string `json:"outcome"`
	IsAmbiguous         bool   `json:"isAmbiguous"`
	AmbiguityReason     string `json:"ambiguityReason,omitempty"`
}

// ParseResult holds the result of parsing free-form text
type ParseResult struct {
	Tasks         []ParsedTask `json:"tasks"`
	Projects      []string     `json:"projects"`
	Conclusion    string       `json:"conclusion,omitempty"`
	EmployeeName  string       `json:"employeeName,omitempty"`
	EmployeeCode  string       `json:"employeeCode,omitempty"`
	AppraiserName string       `json:"appraiserName,omitempty"`
}

// FullMonthDraft stores the full-month text entry
type FullMonthDraft struct {
	Month       string `json:"month"` // YYYY-MM
	RawText     string `json:"rawText"`
	ParsedTasks []Task `json:"parsedTasks,omitempty"`
	SavedAt     string `json:"savedAt"`
}

// CustomSubheading represents an optional user-defined subheading in a task
type CustomSubheading struct {
	Heading string `json:"heading"`
	Content string `json:"content"`
}

// ReportTask is a task as it appears in a report
type ReportTask struct {
	TaskID              string             `json:"taskId"`
	Title               string             `json:"title"`
	IssueIdentified     string             `json:"issueIdentified"`
	ActivitiesPerformed string             `json:"activitiesPerformed"`
	SolutionImplemented string             `json:"solutionImplemented"`
	Outcome             string             `json:"outcome"`
	ChallengesFaced     string             `json:"challengesFaced,omitempty"`
	CustomSections      []CustomSubheading `json:"customSections,omitempty"`
	Order               int                `json:"order"`
}

// ReportSection groups tasks under one project
type ReportSection struct {
	Project string       `json:"project"`
	Tasks   []ReportTask `json:"tasks"`
	Order   int          `json:"order"`
}

// CoverPage holds report cover page fields
type CoverPage struct {
	Title         string   `json:"title"`
	Month         string   `json:"month"`
	Year          string   `json:"year"`
	MonthYear     string   `json:"monthYear"`
	EmployeeName  string   `json:"employeeName"`
	EmployeeCode  string   `json:"employeeCode"`
	AppraiserName string   `json:"appraiserName"`
	Department    string   `json:"department"`
	Projects      []string `json:"projects"`
}

// Report represents a complete monthly performance report
type Report struct {
	ID             string          `json:"id"`
	Month          string          `json:"month"` // YYYY-MM
	TemplateID     string          `json:"templateId,omitempty"`
	FormattingMode string          `json:"formattingMode,omitempty"` // match_template, custom
	SchemaVersion  int             `json:"schemaVersion,omitempty"`
	CoverPage      CoverPage       `json:"coverPage"`
	Sections       []ReportSection `json:"sections"`
	Conclusion     string          `json:"conclusion"`
	Version        int             `json:"version"`
	Status         string          `json:"status"` // draft, final
	GeneratedAt    string          `json:"generatedAt"`
	UpdatedAt      string          `json:"updatedAt"`
}

// ReportVersion is a snapshot of a report
type ReportVersion struct {
	Version     int    `json:"version"`
	SavedAt     string `json:"savedAt"`
	Description string `json:"description"`
}

// Settings holds application settings
type Settings struct {
	AIEnabled       bool   `json:"aiEnabled"`
	AIProvider      string `json:"aiProvider"`
	AIModel         string `json:"aiModel"`
	GitLabEnabled   bool   `json:"gitlabEnabled"`
	GitLabURL       string `json:"gitlabUrl"`
	AutoBackup      bool   `json:"autoBackup"`
	BackupFrequency string `json:"backupFrequency"` // daily, weekly
	DataPath        string `json:"dataPath"`
	UpdatedAt       string `json:"updatedAt"`
}

// AIImproveRequest is the payload for AI improvement
type AIImproveRequest struct {
	Text        string `json:"text"`
	Mode        string `json:"mode"`   // improve, expand, activities, issue, solution, outcome, bullets, conclusion
	Detail      string `json:"detail"` // concise, standard, detailed
	Context     string `json:"context"`
}

// AIImproveResponse is the AI improvement result
type AIImproveResponse struct {
	Original  string `json:"original"`
	Improved  string `json:"improved"`
	Mode      string `json:"mode"`
}

// TaskStats holds dashboard statistics
type TaskStats struct {
	TotalTasks       int      `json:"totalTasks"`
	DaysWithEntries  int      `json:"daysWithEntries"`
	CompletedTasks   int      `json:"completedTasks"`
	Projects         []string `json:"projects"`
	LatestReport     string   `json:"latestReport"`
	RecentTasks      []Task   `json:"recentTasks"`
}

// TextStyle defines typography for a document element in actual pt units
type TextStyle struct {
	FontFamily string  `json:"fontFamily"`
	FontSizePt float64 `json:"fontSizePt"`
	Bold       bool    `json:"bold"`
	Italic     bool    `json:"italic,omitempty"`
	Color      string  `json:"color"`
	Align      string  `json:"align,omitempty"` // left, center, right, justify
}

// TemplatePageConfig holds physical page dimensions and spacing rules
type TemplatePageConfig struct {
	Size               string  `json:"size"`        // A4, Letter
	Orientation        string  `json:"orientation"` // portrait, landscape
	WidthMM            float64 `json:"widthMM"`
	HeightMM           float64 `json:"heightMM"`
	MarginTopMM        float64 `json:"marginTopMM"`
	MarginBottomMM     float64 `json:"marginBottomMM"`
	MarginLeftMM       float64 `json:"marginLeftMM"`
	MarginRightMM      float64 `json:"marginRightMM"`
	LineSpacing        float64 `json:"lineSpacing"`
	ParagraphSpacingPt float64 `json:"paragraphSpacingPt"`
	BulletIndentMM     float64 `json:"bulletIndentMM"`
	ShowPageNumbers    bool    `json:"showPageNumbers"`
	PageNumberStyle    string  `json:"pageNumberStyle"` // badge, plain, none
	FooterText         string  `json:"footerText"`      // e.g. "Public"
}

// TemplateTypography holds per-element styles
type TemplateTypography struct {
	BannerTitle     TextStyle `json:"bannerTitle"`
	CoverProjects   TextStyle `json:"coverProjects"`
	CoverEmployee   TextStyle `json:"coverEmployee"`
	TOCTitle        TextStyle `json:"tocTitle"`
	TOCProject      TextStyle `json:"tocProject"`
	TOCTask         TextStyle `json:"tocTask"`
	ProjectName     TextStyle `json:"projectName"`
	TaskTitle       TextStyle `json:"taskTitle"`
	Subheading      TextStyle `json:"subheading"`
	TaskDescription TextStyle `json:"taskDescription"`
	ConclusionTitle TextStyle `json:"conclusionTitle"`
	ConclusionBody  TextStyle `json:"conclusionBody"`
}

// TemplateAssets stores template-isolated asset filenames under data/templates/<id>/assets/
type TemplateAssets struct {
	CoverHeroImage  string   `json:"coverHeroImage,omitempty"`
	LogoImage       string   `json:"logoImage,omitempty"`
	HeaderImage     string   `json:"headerImage,omitempty"`
	FooterImage     string   `json:"footerImage,omitempty"`
	BackgroundImage string   `json:"backgroundImage,omitempty"`
	ExtractedFiles  []string `json:"extractedFiles,omitempty"`
}

// TemplateFieldMapping maps a logical report field to a template region/style
type TemplateFieldMapping struct {
	ID           string  `json:"id"`
	FieldType    string  `json:"fieldType"` // cover_page, employee_name, employee_code, appraiser_name, month, year, projects, toc_area, task_title, task_description, repeatable_task, header, footer, page_number, background_image
	Label        string  `json:"label"`
	PageNumber   int     `json:"pageNumber"`
	X            float64 `json:"x"`      // % or mm
	Y            float64 `json:"y"`      // % or mm
	Width        float64 `json:"width"`  // % or mm
	Height       float64 `json:"height"` // % or mm
	FontFamily   string  `json:"fontFamily"`
	FontSizePt   float64 `json:"fontSizePt"`
	Bold         bool    `json:"bold"`
	Color        string  `json:"color"`
	MaskExisting bool    `json:"maskExisting,omitempty"`
}

// TemplateAnalysisResult summarizes auto-detected properties of an imported PDF/DOCX
type TemplateAnalysisResult struct {
	PageCount        int       `json:"pageCount"`
	PageWidthPt      float64   `json:"pageWidthPt"`
	PageHeightPt     float64   `json:"pageHeightPt"`
	Orientation      string    `json:"orientation"`
	MarginTopPt      float64   `json:"marginTopPt"`
	MarginBottomPt   float64   `json:"marginBottomPt"`
	MarginLeftPt     float64   `json:"marginLeftPt"`
	MarginRightPt    float64   `json:"marginRightPt"`
	Fonts            []string  `json:"fonts"`
	FontSizesPt      []float64 `json:"fontSizesPt"`
	Colors           []string  `json:"colors"`
	ImagesCount      int       `json:"imagesCount"`
	HasTOC           bool      `json:"hasTOC"`
	DetectedHeadings []string  `json:"detectedHeadings"`
	Notes            []string  `json:"notes"`
}

// Template represents a report template stored under data/templates/<id>/template.json
type Template struct {
	ID             string                  `json:"id"`
	Name           string                  `json:"name"`
	Description    string                  `json:"description"`
	IsDefault      bool                    `json:"isDefault"`
	SourceType     string                  `json:"sourceType"` // era-builtin, docx, pdf
	SourceFile     string                  `json:"sourceFile,omitempty"`
	FormattingMode string                  `json:"formattingMode"` // match_template, custom
	Approved       bool                    `json:"approved"`
	SchemaVersion  int                     `json:"schemaVersion,omitempty"`
	PrimaryColor   string                  `json:"primaryColor"`
	AccentColor    string                  `json:"accentColor"`
	LogoPath       string                  `json:"logoPath,omitempty"`
	Sections       []string                `json:"sections"`
	PageConfig     TemplatePageConfig      `json:"pageConfig"`
	Typography     TemplateTypography      `json:"typography"`
	Assets         TemplateAssets          `json:"assets"`
	FieldMappings  []TemplateFieldMapping  `json:"fieldMappings,omitempty"`
	DetectedInfo   *TemplateAnalysisResult `json:"detectedInfo,omitempty"`
	CreatedAt      string                  `json:"createdAt"`
	UpdatedAt      string                  `json:"updatedAt"`
}

// GitLabCommit represents an imported GitLab commit
type GitLabCommit struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	AuthorName string `json:"authorName"`
	CommittedDate string `json:"committedDate"`
	WebURL    string `json:"webUrl"`
	ProjectName string `json:"projectName"`
}

// BackupInfo holds information about a backup
type BackupInfo struct {
	Filename  string `json:"filename"`
	CreatedAt string `json:"createdAt"`
	SizeMB    float64 `json:"sizeMB"`
}

// BulkTaskRequest holds tasks to import in bulk
type BulkTaskRequest struct {
	Month string `json:"month"` // YYYY-MM
	Tasks []Task `json:"tasks"`
}

// EditorPageSettings stores page dimensions, margins, and header/footer text for the document editor
type EditorPageSettings struct {
	PageSize      string  `json:"pageSize"`      // "Letter" or "A4"
	MarginTopMM   float64 `json:"marginTopMM"`   // e.g. 12.7
	MarginRightMM float64 `json:"marginRightMM"` // e.g. 20.32
	MarginBottomMM float64 `json:"marginBottomMM"`
	MarginLeftMM  float64 `json:"marginLeftMM"`
	HeaderText    string  `json:"headerText"`
	FooterText    string  `json:"footerText"`
}

// EditedDocument stores the manually edited report HTML and page settings
type EditedDocument struct {
	Month        string             `json:"month"`
	HTMLContent  string             `json:"htmlContent"`
	PageSettings EditorPageSettings `json:"pageSettings"`
	IsDraft      bool               `json:"isDraft"`
	UpdatedAt    string             `json:"updatedAt"`
	SourceMode   string             `json:"sourceMode"` // "editor" or "word"
}

// EditorStatus reports whether draft or saved manual edits exist for a month
type EditorStatus struct {
	HasDraft       bool   `json:"hasDraft"`
	HasSavedEdits  bool   `json:"hasSavedEdits"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
	DraftUpdatedAt string `json:"draftUpdatedAt,omitempty"`
	SourceMode     string `json:"sourceMode,omitempty"`
}

// MonthlyReportStatus tracks lifecycle state and reminders for a month
type MonthlyReportStatus struct {
	Month            string  `json:"month"`
	Status           string  `json:"status"` // IN_PROGRESS, GENERATED_DRAFT, EDITED_DRAFT, COMPLETED
	GeneratedAt      *string `json:"generatedAt"`
	EditedAt         *string `json:"editedAt"`
	CompletedAt      *string `json:"completedAt"`
	LastReminderAt   *string `json:"lastReminderAt"`
	RemindLaterUntil *string `json:"remindLaterUntil"`
	PDFPath          *string `json:"pdfPath"`
	DOCXPath         *string `json:"docxPath"`
	TasksUpdatedAt   *string `json:"tasksUpdatedAt,omitempty"`
}

// MissingTaskSummary summarizes entered vs missing dates for a month
type MissingTaskSummary struct {
	Month               string   `json:"month"`
	MonthLabel          string   `json:"monthLabel"`
	TotalCalendarDays   int      `json:"totalCalendarDays"`
	DaysWithEntries     int      `json:"daysWithEntries"`
	TotalTasks          int      `json:"totalTasks"`
	DatesWithoutEntries []string `json:"datesWithoutEntries"`
}

// ReminderEvaluation holds startup/midnight reminder & last-day state
type ReminderEvaluation struct {
	CurrentDate         string                `json:"currentDate"`
	CurrentMonth        string                `json:"currentMonth"`
	CurrentMonthName    string                `json:"currentMonthName"`
	IsLastDayOfMonth    bool                  `json:"isLastDayOfMonth"`
	CurrentMonthStatus  MonthlyReportStatus   `json:"currentMonthStatus"`
	PendingReminder     *PendingMonthReminder `json:"pendingReminder,omitempty"`
	DraftNotice         *PendingMonthReminder `json:"draftNotice,omitempty"`
	TasksChangedWarning bool                  `json:"tasksChangedWarning"`
}

type PendingMonthReminder struct {
	Month      string `json:"month"`
	MonthName  string `json:"monthName"`
	MonthLabel string `json:"monthLabel"`
	Status     string `json:"status"`
	TaskCount  int    `json:"taskCount"`
}
