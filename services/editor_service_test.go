package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"performancereport/models"
	"performancereport/storage"
)

func TestEditorLifecycle_DraftSaveDiscardAndBackup(t *testing.T) {
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

	month := "2026-11"
	report := &models.Report{
		ID:             "rep-1",
		Month:          month,
		TemplateID:     "era",
		FormattingMode: "match",
		CoverPage: models.CoverPage{
			MonthYear:     "November 2026",
			EmployeeName:  "Sham Anil Patil",
			EmployeeCode:  "1947",
			AppraiserName: "Shivprakash Bapat",
		},
		Sections: []models.ReportSection{
			{
				Project: "EMACloudContentManager",
				Order:   1,
				Tasks: []models.ReportTask{
					{
						TaskID:              "t1",
						Title:               "Initial Task",
						ActivitiesPerformed: "- Initial activity line.",
						Outcome:             "Initial outcome.",
					},
				},
			},
		},
		Conclusion: "Initial conclusion.",
	}
	if err := SaveReport(report); err != nil {
		t.Fatalf("SaveReport failed: %v", err)
	}

	// 1. Get initial editor document
	doc, st, err := GetEditorDocument(month)
	if err != nil {
		t.Fatalf("GetEditorDocument failed: %v", err)
	}
	if st.HasDraft || st.HasSavedEdits {
		t.Fatalf("expected clean initial status, got %+v", st)
	}
	if !strings.Contains(doc.HTMLContent, "EMACloudContentManager") {
		t.Fatalf("expected initial HTML to include project name")
	}

	// 2. Save Draft
	doc.HTMLContent = `<h1 style="font-family: 'Century Gothic'; font-size: 23pt;">EMACloudContentManager</h1><p style="text-align: justify;">Edited manually in Word-like editor.</p><div data-page-break="true"><span>Manual Page Break</span></div><p>Paragraph on next page.</p><table><tr><th>Col1</th></tr><tr><td>Val1</td></tr></table>`
	doc.PageSettings.PageSize = "A4"
	doc.PageSettings.MarginLeftMM = 25
	if err := SaveEditorDraft(month, doc); err != nil {
		t.Fatalf("SaveEditorDraft failed: %v", err)
	}
	st = GetEditorStatus(month)
	if !st.HasDraft || st.HasSavedEdits {
		t.Fatalf("expected HasDraft=true, HasSavedEdits=false, got %+v", st)
	}

	// 3. Save Changes permanently -> creates edited_report.docx & clears draft
	st, err = SaveEditorChanges(month, doc)
	if err != nil {
		t.Fatalf("SaveEditorChanges failed: %v", err)
	}
	if st.HasDraft || !st.HasSavedEdits {
		t.Fatalf("expected HasDraft=false, HasSavedEdits=true after SaveChanges, got %+v", st)
	}
	editedDocx := storage.ReportEditedDocxPath(month)
	if !storage.FileExists(editedDocx) {
		t.Fatalf("expected edited_report.docx at %s", editedDocx)
	}
	xmlStr := readDocxDocumentXML(t, editedDocx)
	if !strings.Contains(xmlStr, "Edited manually in Word-like editor.") || !strings.Contains(xmlStr, `<w:br w:type="page"/>`) || !strings.Contains(xmlStr, "<w:tbl>") {
		t.Fatalf("edited DOCX missing manual text, page break, or table")
	}
	if !strings.Contains(xmlStr, `w:w="11906" w:h="16838"`) {
		t.Fatalf("expected A4 page size (11906x16838) in edited DOCX")
	}

	// 4. Save another draft, then Discard -> restores permanently saved version
	draft2 := *doc
	draft2.HTMLContent = "<p>Temporary unsaved draft</p>"
	if err := SaveEditorDraft(month, &draft2); err != nil {
		t.Fatalf("SaveEditorDraft(2) failed: %v", err)
	}
	restored, stAfterDiscard, err := DiscardEditorChanges(month)
	if err != nil {
		t.Fatalf("DiscardEditorChanges failed: %v", err)
	}
	if stAfterDiscard.HasDraft || !stAfterDiscard.HasSavedEdits {
		t.Fatalf("expected draft cleared and saved edits preserved after discard, got %+v", stAfterDiscard)
	}
	if !strings.Contains(restored.HTMLContent, "Edited manually in Word-like editor.") {
		t.Fatalf("expected restored content to match last saved version")
	}

	// 5. BackupAndClearEditedOnRegenerate -> backs up before clearing
	if err := BackupAndClearEditedOnRegenerate(month); err != nil {
		t.Fatalf("BackupAndClearEditedOnRegenerate failed: %v", err)
	}
	if storage.FileExists(editedDocx) {
		t.Fatalf("expected edited_report.docx cleared after explicit regenerate")
	}
	backups, _ := filepath.Glob(filepath.Join(storage.ReportEditedDir(month), "backups", "*.bak"))
	if len(backups) == 0 {
		t.Fatalf("expected backup files to be preserved in edited/backups/")
	}
}
