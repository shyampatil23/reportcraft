package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"performancereport/models"
	"performancereport/services"
	"performancereport/storage"
)

func ListReports(w http.ResponseWriter, r *http.Request) {
	reports, err := services.ListReports()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, reports)
}

func GetReport(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	report, err := services.GetReport(month)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if report == nil {
		jsonError(w, "report not found", 404)
		return
	}
	jsonOK(w, report)
}

func GenerateReport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Month               string   `json:"month"`
		SelectedTaskIDs     []string `json:"selectedTaskIds"`
		TemplateID          string   `json:"templateId"`
		FormattingMode      string   `json:"formattingMode"`
		ForceOverwriteEdits bool     `json:"forceOverwriteEdits"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body: "+err.Error(), 400)
		return
	}
	if req.Month == "" {
		jsonError(w, "month is required", 400)
		return
	}
	if req.ForceOverwriteEdits {
		_ = services.BackupAndClearEditedOnRegenerate(req.Month)
	}
	report, err := services.GenerateReportWithOptions(req.Month, req.SelectedTaskIDs, req.TemplateID, req.FormattingMode)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, report)
}

func UpdateReport(w http.ResponseWriter, r *http.Request) {
	var report models.Report
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		jsonError(w, "invalid body: "+err.Error(), 400)
		return
	}
	if err := services.SaveReport(&report); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, report)
}

func ExportPDF(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	report, err := services.GetReport(month)
	if err != nil || report == nil {
		jsonError(w, "report not found", 404)
		return
	}
	path, err := services.GeneratePDF(report)
	if err != nil {
		jsonError(w, "PDF generation failed: "+err.Error(), 500)
		return
	}
	// Serve the HTML file (which is print-to-PDF compatible)
	http.ServeFile(w, r, path)
}

func ExportDocx(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	report, err := services.GetReport(month)
	if err != nil || report == nil {
		jsonError(w, "report not found", 404)
		return
	}
	path := storage.ReportEditedDocxPath(month)
	downloadName := ""
	if storage.FileExists(path) {
		if t, parseErr := time.Parse("2006-01", month); parseErr == nil {
			downloadName = fmt.Sprintf("PerformanceReport%s.docx", t.Format("January2006"))
		} else {
			downloadName = filepath.Base(path)
		}
	} else {
		path, err = services.GenerateDocx(report)
		if err != nil {
			jsonError(w, "DOCX generation failed: "+err.Error(), 500)
			return
		}
		downloadName = filepath.Base(path)
	}
	// Stream the file
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", `attachment; filename="`+downloadName+`"`)
	http.ServeFile(w, r, path)
}

func ExportPDFDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	report, err := services.GetReport(month)
	if err != nil || report == nil {
		jsonError(w, "report not found", 404)
		return
	}
	path := storage.ReportEditedPDFPath(month)
	downloadName := ""
	if storage.FileExists(path) {
		if t, parseErr := time.Parse("2006-01", month); parseErr == nil {
			downloadName = fmt.Sprintf("PerformanceReport%s.pdf", t.Format("January2006"))
		} else {
			downloadName = filepath.Base(path)
		}
	} else {
		path, err = services.GeneratePDF(report)
		if err != nil {
			jsonError(w, "PDF generation failed: "+err.Error(), 500)
			return
		}
		downloadName = filepath.Base(path)
	}
	if dir := strings.TrimSpace(r.URL.Query().Get("dir")); dir != "" {
		_ = storage.CopyFile(path, filepath.Join(dir, downloadName))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		jsonError(w, "file read error: "+err.Error(), 500)
		return
	}
	if strings.HasSuffix(strings.ToLower(path), ".pdf") {
		w.Header().Set("Content-Type", "application/pdf")
	} else {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	disp := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disp = "inline"
	}
	w.Header().Set("Content-Disposition", disp+`; filename="`+downloadName+`"`)
	w.Write(data)
}

func GetReportHTML(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	report, err := services.GetReport(month)
	if err != nil || report == nil {
		jsonError(w, "report not found", 404)
		return
	}
	if r.URL.Query().Get("mode") != "html" {
		if editedPDF := storage.ReportEditedPDFPath(month); storage.FileExists(editedPDF) {
			if data, readErr := os.ReadFile(editedPDF); readErr == nil {
				w.Header().Set("Content-Type", "application/pdf")
				w.Header().Set("Content-Disposition", `inline; filename="edited_report.pdf"`)
				w.Write(data)
				return
			}
		}
		if pdfPath, pdfErr := services.GeneratePDF(report); pdfErr == nil && strings.HasSuffix(strings.ToLower(pdfPath), ".pdf") {
			if data, readErr := os.ReadFile(pdfPath); readErr == nil {
				w.Header().Set("Content-Type", "application/pdf")
				w.Header().Set("Content-Disposition", `inline; filename="`+filepath.Base(pdfPath)+`"`)
				w.Write(data)
				return
			}
		}
	}
	path, err := services.GenerateHTMLPreview(report)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func GetTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := services.GetTemplates()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, templates)
}

func ImportTemplate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		jsonError(w, "failed to parse upload: "+err.Error(), 400)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "template file is required: "+err.Error(), 400)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		jsonError(w, "failed to read file: "+err.Error(), 500)
		return
	}

	name := r.FormValue("name")
	primaryColor := r.FormValue("primaryColor")
	accentColor := r.FormValue("accentColor")

	tmpl, err := services.ImportTemplateFile(name, primaryColor, accentColor, header.Filename, data)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	jsonOK(w, tmpl)
}

func UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var tmpl models.Template
	if err := json.NewDecoder(r.Body).Decode(&tmpl); err != nil {
		jsonError(w, "invalid body: "+err.Error(), 400)
		return
	}
	tmpl.ID = id
	if err := services.SaveTemplate(&tmpl); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, tmpl)
}

func DuplicateTemplate(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	clone, err := services.DuplicateTemplate(id)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	jsonOK(w, clone)
}

func DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := services.DeleteTemplate(id); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

func SetDefaultTemplate(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	tmpl, err := services.SetDefaultTemplate(id)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	jsonOK(w, tmpl)
}

func GetTemplateSource(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	tmpl, err := services.GetTemplate(id)
	if err != nil || tmpl == nil {
		jsonError(w, "template not found", 404)
		return
	}
	if tmpl.SourceFile == "" {
		jsonError(w, "template has no uploaded source file", 404)
		return
	}
	srcPath := filepath.Join(storage.TemplateDir(tmpl.ID), filepath.Base(tmpl.SourceFile))
	if !storage.FileExists(srcPath) {
		jsonError(w, "source file missing", 404)
		return
	}
	if strings.HasSuffix(strings.ToLower(tmpl.SourceFile), ".pdf") {
		w.Header().Set("Content-Type", "application/pdf")
	} else {
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	}
	http.ServeFile(w, r, srcPath)
}

func GetTemplateAsset(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	filename := filepath.Base(vars["filename"])
	assetPath := filepath.Join(storage.TemplateAssetsDir(id), filename)
	if !storage.FileExists(assetPath) {
		jsonError(w, "asset not found", 404)
		return
	}
	http.ServeFile(w, r, assetPath)
}

func GetTemplatePreviewHTML(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	tmpl, err := services.GetTemplate(id)
	if err != nil || tmpl == nil {
		jsonError(w, "template not found", 404)
		return
	}
	sample := &models.Report{
		ID:             "sample-" + tmpl.ID,
		Month:          time.Now().Format("2006-01"),
		TemplateID:     tmpl.ID,
		FormattingMode: tmpl.FormattingMode,
		CoverPage: models.CoverPage{
			Title:         "Monthly Performance Report",
			Month:         time.Now().Format("January"),
			Year:          time.Now().Format("2006"),
			MonthYear:     time.Now().Format("January 2006"),
			EmployeeName:  "Sample Employee",
			EmployeeCode:  "EMP001",
			AppraiserName: "Appraiser Name",
			Projects:      []string{"Project Alpha", "Project Beta"},
		},
		Sections: []models.ReportSection{
			{
				Project: "Project Alpha",
				Tasks: []models.ReportTask{
					{
						Title:               "Sample Feature Implementation & Verification",
						IssueIdentified:     "Existing workflow lacked automated validation.",
						ActivitiesPerformed: "Implemented validation service.\nAdded unit tests and verified edge cases.",
						SolutionImplemented: "Modularized validator and integrated into pipeline.",
						Outcome:             "Reduced manual review overhead and improved reliability.",
					},
				},
			},
		},
		Conclusion: "All planned deliverables for this period were completed and verified.",
	}
	html := services.BuildPDFContent(sample)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func GetSettings(w http.ResponseWriter, r *http.Request) {
	s, err := services.GetSettings()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, s)
}

func UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var s models.Settings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		jsonError(w, "invalid body: "+err.Error(), 400)
		return
	}
	if err := services.SaveSettings(&s); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, s)
}

func CreateBackup(w http.ResponseWriter, r *http.Request) {
	info, err := services.CreateBackup()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, info)
}

func ListBackups(w http.ResponseWriter, r *http.Request) {
	backups, err := services.ListBackups()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, backups)
}

func OpenDataFolder(w http.ResponseWriter, r *http.Request) {
	dataPath := storage.DataPath()
	storage.EnsureDir(dataPath)
	jsonOK(w, map[string]string{"path": dataPath})
}

func ImproveWithAI(w http.ResponseWriter, r *http.Request) {
	var req models.AIImproveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body: "+err.Error(), 400)
		return
	}
	resp, err := services.ImproveText(&req)
	if err != nil {
		jsonError(w, "AI error: "+err.Error(), 503)
		return
	}
	jsonOK(w, resp)
}

func GetAICreds(w http.ResponseWriter, r *http.Request) {
	creds, err := services.GetAICreds()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, creds)
}

func TestAIConnection(w http.ResponseWriter, r *http.Request) {
	result, err := services.TestAIConnection()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, result)
}

func SaveAICreds(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
		APIKey   string `json:"apiKey"`
		Model    string `json:"model"`
		BaseURL  string `json:"baseUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if err := services.SaveAICreds(body.Provider, body.APIKey, body.Model, body.BaseURL); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "saved"})
}

func GetGitLabCommits(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project")
	month := r.URL.Query().Get("month")
	commits, err := services.GetGitLabCommits(projectID, month)
	if err != nil {
		jsonError(w, "GitLab error: "+err.Error(), 503)
		return
	}
	jsonOK(w, commits)
}

func GetEditorDocument(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	doc, status, err := services.GetEditorDocument(month)
	if err != nil {
		jsonError(w, err.Error(), 404)
		return
	}
	jsonOK(w, map[string]interface{}{
		"document": doc,
		"status":   status,
	})
}

func GetEditorStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	jsonOK(w, services.GetEditorStatus(month))
}

func SaveEditorDraft(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	var doc models.EditedDocument
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		jsonError(w, "invalid body: "+err.Error(), 400)
		return
	}
	if err := services.SaveEditorDraft(month, &doc); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, services.GetEditorStatus(month))
}

func SaveEditorChanges(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	var doc models.EditedDocument
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		jsonError(w, "invalid body: "+err.Error(), 400)
		return
	}
	status, err := services.SaveEditorChanges(month, &doc)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, status)
}

func DiscardEditorChanges(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	doc, status, err := services.DiscardEditorChanges(month)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]interface{}{
		"document": doc,
		"status":   status,
	})
}

func OpenInMicrosoftWord(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	path, err := services.OpenInMicrosoftWord(month)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{
		"status": "opened",
		"path":   path,
	})
}

func SyncFromMicrosoftWord(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	status, err := services.SyncFromMicrosoftWord(month)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, status)
}

func ClearEditedOnRegenerate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	if err := services.BackupAndClearEditedOnRegenerate(month); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, services.GetEditorStatus(month))
}

func GetMonthlyReminders(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	if dateStr := strings.TrimSpace(r.URL.Query().Get("date")); dateStr != "" {
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			now = parsed
		}
	}
	eval, err := services.EvaluateMonthlyReminders(now)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, eval)
}

func GetMonthStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	st, err := services.GetMonthlyReportStatus(month)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]interface{}{
		"statusInfo":          st,
		"tasksChangedWarning": services.CheckTasksModifiedAfterGeneration(month),
	})
}

func RemindLaterReport(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	st, err := services.RemindLaterReport(month, 24)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, st)
}

func CompleteReport(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	st, err := services.MarkReportCompleted(month)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, st)
}

func GetMissingTaskSummary(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	summary, err := services.GetMissingTaskSummary(month)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, summary)
}

func GetMonthHistory(w http.ResponseWriter, r *http.Request) {
	items, err := services.ListAllMonthHistory()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, items)
}
