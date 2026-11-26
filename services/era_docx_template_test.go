package services

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"performancereport/models"
)

func TestGenerateDocxFromERAMasterTemplate_MatchAndCustom(t *testing.T) {
	tmpDir := t.TempDir()

	report := &models.Report{
		Month:          "2026-11",
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
						Title:               "Translated Video Integration API – Payload and QC Enhancements",
						ActivitiesPerformed: "- Enhanced the API to support processing multiple\ntranslated videos in a single request.\n- Added validation for missing QC details.",
						Outcome:             "Enabled batch processing of translated videos with improved reliability.",
					},
				},
			},
			{
				Project: "ERA 3.0",
				Order:   2,
				Tasks: []models.ReportTask{
					{
						TaskID:              "t2",
						Title:               "Simulation Attempt Feedback Smileys",
						ActivitiesPerformed: "- Replaced text-only indicators with smiley feedback icons.",
						Outcome:             "Improved learner clarity during simulation attempts.",
					},
				},
			},
		},
		Conclusion: "Overall, November 2026 improvements enhanced batch integration and learner experience.",
	}

	// 1. Test Match Original Template mode
	matchOut := filepath.Join(tmpDir, "match.docx")
	eraTmpl := DefaultERATemplate()
	if err := generateDocxFromMasterTemplate(report, &eraTmpl, matchOut); err != nil {
		t.Fatalf("generateDocxFromMasterTemplate(match) failed: %v", err)
	}
	matchXML := readDocxDocumentXML(t, matchOut)

	// Cover page placeholders replaced
	if !strings.Contains(matchXML, "Sham Anil Patil") || !strings.Contains(matchXML, "1947") || !strings.Contains(matchXML, "Shivprakash Bapat") {
		t.Fatalf("cover page employee fields not updated")
	}
	if !strings.Contains(matchXML, "November 2026") || strings.Contains(matchXML, "October 2026") {
		t.Fatalf("cover page month/year not updated to November 2026")
	}
	if !strings.Contains(matchXML, "EMACloudContentManager, ERA 3.0") {
		t.Fatalf("cover page projects line not updated")
	}
	// TOC preserved without page numbers or dotted leaders
	if !strings.Contains(matchXML, "TABLE OF CONTENTS") || !strings.Contains(matchXML, "Rectangle 31") {
		t.Fatalf("golden TOC heading/background missing")
	}
	if strings.Contains(matchXML, `w:leader="dot"`) || strings.Contains(matchXML, "PAGEREF") {
		t.Fatalf("TOC should not contain page numbers or dotted leaders")
	}
	// Headings have keepNext
	if !strings.Contains(matchXML, "<w:keepNext/>") {
		t.Fatalf("headings should include <w:keepNext/> to prevent orphan headings")
	}
	// Match mode uses original 26pt (52) for project heading
	if !strings.Contains(matchXML, `<w:sz w:val="52"/>`) {
		t.Fatalf("match mode should use w:sz=52 (26pt) for project heading")
	}
	// Preserves header/footer/US Letter sectPr
	if !strings.Contains(matchXML, `w:w="12240" w:h="15840"`) {
		t.Fatalf("US Letter sectPr page size not preserved")
	}

	// 2. Test Custom Formatting mode (23pt Century Gothic / 16pt TNR / 14pt TNR)
	report.FormattingMode = "custom"
	tmplCustom := DefaultERATemplate()
	tmplCustom.FormattingMode = "custom"
	customOut := filepath.Join(tmpDir, "custom.docx")
	if err := generateDocxFromMasterTemplate(report, &tmplCustom, customOut); err != nil {
		t.Fatalf("generateDocxFromMasterTemplate(custom) failed: %v", err)
	}
	customXML := readDocxDocumentXML(t, customOut)
	if !strings.Contains(customXML, `Century Gothic`) || !strings.Contains(customXML, `<w:sz w:val="46"/>`) {
		t.Fatalf("custom mode should use Century Gothic 23pt (w:sz=46) for project heading")
	}
	if !strings.Contains(customXML, `<w:sz w:val="32"/>`) || !strings.Contains(customXML, `<w:sz w:val="28"/>`) {
		t.Fatalf("custom mode should use 16pt (32) for task title and 14pt (28) for description/subheading")
	}

	// Ensure file is non-trivial
	if fi, err := os.Stat(matchOut); err != nil || fi.Size() < 100000 {
		t.Fatalf("expected master-based docx to preserve embedded media (>100KB), got %v", err)
	}
}

func readDocxDocumentXML(t *testing.T, docxPath string) string {
	t.Helper()
	zr, err := zip.OpenReader(docxPath)
	if err != nil {
		t.Fatalf("open zip %s: %v", docxPath, err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open document.xml: %v", err)
			}
			defer rc.Close()
			b, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read document.xml: %v", err)
			}
			return string(b)
		}
	}
	t.Fatalf("word/document.xml not found in %s", docxPath)
	return ""
}
