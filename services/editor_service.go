package services

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"performancereport/models"
	"performancereport/storage"
)

// DefaultEditorPageSettings returns page settings matching the report's effective template.
func DefaultEditorPageSettings(r *models.Report) models.EditorPageSettings {
	tmpl := resolveEffectiveTemplate(r)
	ps := models.EditorPageSettings{
		PageSize:       "Letter",
		MarginTopMM:    12.7,
		MarginRightMM:  20.32,
		MarginBottomMM: 12.7,
		MarginLeftMM:   20.32,
		HeaderText:     "",
		FooterText:     "Public",
	}
	if tmpl != nil && tmpl.ID != "era" {
		if strings.EqualFold(tmpl.PageConfig.Size, "A4") {
			ps.PageSize = "A4"
		}
		if tmpl.PageConfig.MarginTopMM > 0 {
			ps.MarginTopMM = tmpl.PageConfig.MarginTopMM
		}
		if tmpl.PageConfig.MarginRightMM > 0 {
			ps.MarginRightMM = tmpl.PageConfig.MarginRightMM
		}
		if tmpl.PageConfig.MarginBottomMM > 0 {
			ps.MarginBottomMM = tmpl.PageConfig.MarginBottomMM
		}
		if tmpl.PageConfig.MarginLeftMM > 0 {
			ps.MarginLeftMM = tmpl.PageConfig.MarginLeftMM
		}
		if strings.TrimSpace(tmpl.PageConfig.FooterText) != "" {
			ps.FooterText = tmpl.PageConfig.FooterText
		}
	}
	return ps
}

// GetEditorStatus returns whether draft or permanently saved manual edits exist for a month.
func GetEditorStatus(month string) models.EditorStatus {
	var st models.EditorStatus
	if storage.FileExists(storage.ReportEditorDraftPath(month)) {
		st.HasDraft = true
		var d models.EditedDocument
		if err := storage.ReadJSON(storage.ReportEditorDraftPath(month), &d); err == nil {
			st.DraftUpdatedAt = d.UpdatedAt
		}
	}
	if storage.FileExists(storage.ReportEditorSavedPath(month)) || storage.FileExists(storage.ReportEditedDocxPath(month)) {
		st.HasSavedEdits = true
		var s models.EditedDocument
		if err := storage.ReadJSON(storage.ReportEditorSavedPath(month), &s); err == nil {
			st.UpdatedAt = s.UpdatedAt
			st.SourceMode = s.SourceMode
		}
	}
	return st
}

// GetEditorDocument loads the draft (if present), else saved edits, else generates initial editable HTML from the report.
func GetEditorDocument(month string) (*models.EditedDocument, models.EditorStatus, error) {
	st := GetEditorStatus(month)

	if st.HasDraft {
		var doc models.EditedDocument
		if err := storage.ReadJSON(storage.ReportEditorDraftPath(month), &doc); err == nil && doc.HTMLContent != "" {
			return &doc, st, nil
		}
	}
	if storage.FileExists(storage.ReportEditorSavedPath(month)) {
		var doc models.EditedDocument
		if err := storage.ReadJSON(storage.ReportEditorSavedPath(month), &doc); err == nil && doc.HTMLContent != "" {
			return &doc, st, nil
		}
	}

	report, err := GetReport(month)
	if err != nil || report == nil {
		return nil, st, fmt.Errorf("report not found for %s", month)
	}
	doc := &models.EditedDocument{
		Month:        month,
		HTMLContent:  BuildInitialEditableHTML(report),
		PageSettings: DefaultEditorPageSettings(report),
		IsDraft:      false,
		UpdatedAt:    time.Now().Format(time.RFC3339),
		SourceMode:   "editor",
	}
	return doc, st, nil
}

// BuildInitialEditableHTML converts a Report into clean, Word-styled semantic HTML for the paginated Tiptap editor.
func BuildInitialEditableHTML(r *models.Report) string {
	tmpl := resolveEffectiveTemplate(r)
	isCustom := strings.EqualFold(r.FormattingMode, "custom") || (tmpl != nil && strings.EqualFold(tmpl.FormattingMode, "custom"))

	projPt := 26.0
	projFont := "Century Gothic"
	taskPt := 16.0
	subPt := 15.0
	bodyPt := 14.0
	if isCustom {
		projPt = 23.0
		subPt = 14.0
		if tmpl != nil {
			if tmpl.Typography.ProjectName.FontSizePt > 0 {
				projPt = tmpl.Typography.ProjectName.FontSizePt
			}
			if strings.TrimSpace(tmpl.Typography.ProjectName.FontFamily) != "" {
				projFont = tmpl.Typography.ProjectName.FontFamily
			}
			if tmpl.Typography.TaskTitle.FontSizePt > 0 {
				taskPt = tmpl.Typography.TaskTitle.FontSizePt
			}
			if tmpl.Typography.Subheading.FontSizePt > 0 {
				subPt = tmpl.Typography.Subheading.FontSizePt
			}
			if tmpl.Typography.TaskDescription.FontSizePt > 0 {
				bodyPt = tmpl.Typography.TaskDescription.FontSizePt
			}
		}
	}

	projects := make([]models.ReportSection, len(r.Sections))
	copy(projects, r.Sections)
	sort.Slice(projects, func(i, j int) bool { return projects[i].Order < projects[j].Order })

	var projNames []string
	for _, p := range projects {
		if strings.TrimSpace(p.Project) != "" {
			projNames = append(projNames, strings.TrimSpace(p.Project))
		}
	}
	projectsLine := formatCoverProjectsLine(projNames)
	monthYear := strings.TrimSpace(r.CoverPage.MonthYear)
	if monthYear == "" {
		if t, pe := time.Parse("2006-01", r.Month); pe == nil {
			monthYear = t.Format("January 2006")
		}
	}

	var b strings.Builder

	// Page 1: Editable Cover Page
	b.WriteString(`<h1 style="font-family: 'Century Gothic', sans-serif; font-size: 24pt; color: #3a3363; font-weight: bold;">Monthly Performance Report</h1>`)
	b.WriteString(fmt.Sprintf(`<p style="font-family: 'Century Gothic', sans-serif; font-size: 18pt; color: #3a3363; font-weight: bold;">%s</p>`, html.EscapeString(monthYear)))
	b.WriteString(fmt.Sprintf(`<p style="font-family: 'Times New Roman', serif; font-size: 14pt; font-weight: bold;">Projects : %s</p>`, html.EscapeString(projectsLine)))
	b.WriteString(fmt.Sprintf(`<p style="font-family: 'Times New Roman', serif; font-size: 14pt;">Employee Name: %s</p>`, html.EscapeString(r.CoverPage.EmployeeName)))
	b.WriteString(fmt.Sprintf(`<p style="font-family: 'Times New Roman', serif; font-size: 14pt;">Employee Code: %s</p>`, html.EscapeString(r.CoverPage.EmployeeCode)))
	b.WriteString(fmt.Sprintf(`<p style="font-family: 'Times New Roman', serif; font-size: 14pt;">Appraiser Name: %s</p>`, html.EscapeString(r.CoverPage.AppraiserName)))
	b.WriteString(`<div data-page-break="true" class="page-break-marker" contenteditable="false"><span>Manual Page Break</span></div>`)

	// Page 2: Editable Table of Contents (no page numbers)
	b.WriteString(`<h2 style="font-family: 'Century Gothic', sans-serif; font-size: 20pt; color: #1f1f1f; font-weight: bold;">TABLE OF CONTENTS</h2>`)
	for _, proj := range projects {
		pName := strings.TrimSpace(proj.Project)
		if pName == "" {
			continue
		}
		b.WriteString(fmt.Sprintf(`<p style="font-family: 'Times New Roman', serif; font-size: 16pt; font-weight: bold;">▪ %s</p>`, html.EscapeString(pName)))
		for _, task := range proj.Tasks {
			tTitle := strings.TrimSpace(task.Title)
			if tTitle == "" {
				continue
			}
			b.WriteString(fmt.Sprintf(`<p style="font-family: 'Times New Roman', serif; font-size: 14pt; margin-left: 24px;">• %s</p>`, html.EscapeString(tTitle)))
		}
	}
	if strings.TrimSpace(r.Conclusion) != "" {
		b.WriteString(`<p style="font-family: 'Times New Roman', serif; font-size: 16pt; font-weight: bold;">▪ Conclusion</p>`)
	}
	b.WriteString(`<div data-page-break="true" class="page-break-marker" contenteditable="false"><span>Manual Page Break</span></div>`)

	// Page 3+: Report Body
	for pi, proj := range projects {
		pName := strings.TrimSpace(proj.Project)
		if pName == "" {
			continue
		}
		if pi > 0 {
			b.WriteString(`<p></p>`)
		}
		b.WriteString(fmt.Sprintf(
			`<h1 style="font-family: '%s', sans-serif; font-size: %gpt; color: #262262; font-weight: bold;">%s</h1>`,
			html.EscapeString(projFont), projPt, html.EscapeString(pName),
		))

		for _, task := range proj.Tasks {
			tTitle := strings.TrimSpace(task.Title)
			if tTitle == "" {
				continue
			}
			b.WriteString(fmt.Sprintf(
				`<h2 style="font-family: 'Times New Roman', serif; font-size: %gpt; font-weight: bold; color: #1f2937;">%s</h2>`,
				taskPt, html.EscapeString("• "+tTitle),
			))

			sections := buildStructuredTaskSections(task)
			for _, sec := range sections {
				b.WriteString(fmt.Sprintf(
					`<h3 style="font-family: 'Times New Roman', serif; font-size: %gpt; font-weight: bold; color: #1f2937;">%s</h3>`,
					subPt, html.EscapeString(sec.Heading),
				))

				inList := false
				for _, item := range sec.Items {
					if item.Kind == blockBullet || item.Kind == blockNestedBullet || item.Kind == blockNumberedItem {
						if !inList {
							b.WriteString(`<ul>`)
							inList = true
						}
						txt := item.Text
						if item.Kind == blockNumberedItem && item.NumPrefix != "" {
							txt = item.NumPrefix + " " + txt
						}
						b.WriteString(fmt.Sprintf(
							`<li><p style="font-family: 'Times New Roman', serif; font-size: %gpt; text-align: justify;">%s</p></li>`,
							bodyPt, html.EscapeString(txt),
						))
					} else {
						if inList {
							b.WriteString(`</ul>`)
							inList = false
						}
						b.WriteString(fmt.Sprintf(
							`<p style="font-family: 'Times New Roman', serif; font-size: %gpt; text-align: justify;">%s</p>`,
							bodyPt, html.EscapeString(item.Text),
						))
					}
				}
				if inList {
					b.WriteString(`</ul>`)
				}
			}
		}
	}

	if strings.TrimSpace(r.Conclusion) != "" {
		b.WriteString(fmt.Sprintf(
			`<h1 style="font-family: '%s', sans-serif; font-size: %gpt; color: #262262; font-weight: bold;">Conclusion</h1>`,
			html.EscapeString(projFont), projPt,
		))
		for _, para := range normalizeParagraphs(r.Conclusion) {
			b.WriteString(fmt.Sprintf(
				`<p style="font-family: 'Times New Roman', serif; font-size: %gpt; text-align: justify;">%s</p>`,
				bodyPt, html.EscapeString(para),
			))
		}
	}

	return b.String()
}

// SaveEditorDraft saves the user's in-progress editing draft locally without overwriting the permanent saved version.
func SaveEditorDraft(month string, doc *models.EditedDocument) error {
	if doc == nil {
		return fmt.Errorf("nil document")
	}
	doc.Month = month
	doc.IsDraft = true
	doc.UpdatedAt = time.Now().Format(time.RFC3339)
	if doc.SourceMode == "" {
		doc.SourceMode = "editor"
	}
	return storage.WriteJSON(storage.ReportEditorDraftPath(month), doc)
}

// SaveEditorChanges saves the edited document permanently (with backups), generates edited_report.docx,
// and converts it to edited_report.pdf so preview and downloads reflect the latest manual edits.
func SaveEditorChanges(month string, doc *models.EditedDocument) (models.EditorStatus, error) {
	if doc == nil {
		return models.EditorStatus{}, fmt.Errorf("nil document")
	}
	report, err := GetReport(month)
	if err != nil || report == nil {
		return models.EditorStatus{}, fmt.Errorf("report not found for %s", month)
	}

	if err := storage.EnsureDir(storage.ReportEditedDir(month)); err != nil {
		return models.EditorStatus{}, err
	}

	// Backup previous saved artifacts to prevent accidental data loss
	_ = storage.BackupFile(storage.ReportEditorSavedPath(month))
	_ = storage.BackupFile(storage.ReportEditedDocxPath(month))
	_ = storage.BackupFile(storage.ReportEditedPDFPath(month))

	doc.Month = month
	doc.IsDraft = false
	doc.UpdatedAt = time.Now().Format(time.RFC3339)
	doc.SourceMode = "editor"

	if err := storage.WriteJSON(storage.ReportEditorSavedPath(month), doc); err != nil {
		return models.EditorStatus{}, err
	}
	_ = storage.DeleteFile(storage.ReportEditorDraftPath(month))

	docxPath := storage.ReportEditedDocxPath(month)
	if err := generateDocxFromEditorHTML(report, doc, docxPath); err != nil {
		return models.EditorStatus{}, fmt.Errorf("generate edited docx: %w", err)
	}

	pdfPath := storage.ReportEditedPDFPath(month)
	if convErr := ConvertDocxToPDF(docxPath, pdfPath); convErr != nil {
		// Fallback to headless Edge HTML->PDF using the exact edited HTML + page settings
		_ = renderEditorHTMLToPDF(doc, pdfPath)
	}
	_, _ = MarkReportEdited(month, pdfPath, docxPath)

	return GetEditorStatus(month), nil
}

// DiscardEditorChanges backs up and removes the unsaved draft and returns the most recent permanently saved version
// (or fresh generated HTML if no permanently saved edits exist).
func DiscardEditorChanges(month string) (*models.EditedDocument, models.EditorStatus, error) {
	draftPath := storage.ReportEditorDraftPath(month)
	if storage.FileExists(draftPath) {
		_ = storage.BackupFile(draftPath)
		_ = storage.DeleteFile(draftPath)
	}
	return GetEditorDocument(month)
}

// OpenInMicrosoftWord creates a working copy of the report DOCX (never touching any master template)
// and launches it in locally installed Microsoft Word.
func OpenInMicrosoftWord(month string) (string, error) {
	report, err := GetReport(month)
	if err != nil || report == nil {
		return "", fmt.Errorf("report not found for %s", month)
	}
	if err := storage.EnsureDir(storage.ReportEditedDir(month)); err != nil {
		return "", err
	}

	workingPath := storage.ReportWordWorkingCopyPath(month)
	if storage.FileExists(storage.ReportEditedDocxPath(month)) {
		if err := storage.CopyFile(storage.ReportEditedDocxPath(month), workingPath); err != nil {
			return "", err
		}
	} else {
		genPath, err := GenerateDocx(report)
		if err != nil {
			return "", err
		}
		if err := storage.CopyFile(genPath, workingPath); err != nil {
			return "", err
		}
	}

	absPath, err := filepath.Abs(workingPath)
	if err != nil {
		absPath = workingPath
	}
	cmd := exec.Command("cmd", "/c", "start", "", absPath)
	if err := cmd.Start(); err != nil {
		return absPath, fmt.Errorf("failed to launch Microsoft Word: %w", err)
	}
	return absPath, nil
}

// SyncFromMicrosoftWord promotes the Microsoft Word working copy into the saved edited DOCX & PDF
// so preview and downloads reflect edits made in Microsoft Word.
func SyncFromMicrosoftWord(month string) (models.EditorStatus, error) {
	workingPath := storage.ReportWordWorkingCopyPath(month)
	if !storage.FileExists(workingPath) {
		return GetEditorStatus(month), fmt.Errorf("working copy not found; click 'Open in Microsoft Word' first")
	}

	_ = storage.BackupFile(storage.ReportEditedDocxPath(month))
	_ = storage.BackupFile(storage.ReportEditedPDFPath(month))
	_ = storage.BackupFile(storage.ReportEditorSavedPath(month))

	docxPath := storage.ReportEditedDocxPath(month)
	if err := storage.CopyFile(workingPath, docxPath); err != nil {
		return GetEditorStatus(month), err
	}

	pdfPath := storage.ReportEditedPDFPath(month)
	if err := ConvertDocxToPDF(docxPath, pdfPath); err != nil {
		return GetEditorStatus(month), fmt.Errorf("convert Word document to PDF: %w", err)
	}

	// Preserve existing saved HTML if any, or mark SourceMode="word"
	var doc models.EditedDocument
	_ = storage.ReadJSON(storage.ReportEditorSavedPath(month), &doc)
	doc.Month = month
	doc.IsDraft = false
	doc.SourceMode = "word"
	doc.UpdatedAt = time.Now().Format(time.RFC3339)
	_ = storage.WriteJSON(storage.ReportEditorSavedPath(month), &doc)
	_, _ = MarkReportEdited(month, pdfPath, docxPath)

	return GetEditorStatus(month), nil
}

// BackupAndClearEditedOnRegenerate backs up all manual edits before clearing them on an explicit user-confirmed regenerate.
func BackupAndClearEditedOnRegenerate(month string) error {
	_ = storage.BackupFile(storage.ReportEditorDraftPath(month))
	_ = storage.BackupFile(storage.ReportEditorSavedPath(month))
	_ = storage.BackupFile(storage.ReportEditedDocxPath(month))
	_ = storage.BackupFile(storage.ReportEditedPDFPath(month))
	_ = storage.DeleteFile(storage.ReportEditorDraftPath(month))
	_ = storage.DeleteFile(storage.ReportEditorSavedPath(month))
	_ = storage.DeleteFile(storage.ReportEditedDocxPath(month))
	_ = storage.DeleteFile(storage.ReportEditedPDFPath(month))
	return nil
}

// ─── HTML-to-DOCX Generator ──────────────────────────────────────────────────

type editorEmbeddedImage struct {
	RelID   string
	ZipPath string
	Ext     string
	Bytes   []byte
	DocPrID int
}

func generateDocxFromEditorHTML(r *models.Report, doc *models.EditedDocument, outputPath string) error {
	masterBytes, err := loadERAMasterDocxBytes()
	if err != nil {
		// Fallback: if master template missing, generate base docx first then use it
		basePath, genErr := GenerateDocx(r)
		if genErr != nil {
			return err
		}
		masterBytes, err = os.ReadFile(basePath)
		if err != nil {
			return err
		}
	}

	zr, err := zip.NewReader(bytes.NewReader(masterBytes), int64(len(masterBytes)))
	if err != nil {
		return fmt.Errorf("open master zip: %w", err)
	}

	var embeddedImages []editorEmbeddedImage
	editedCover, editedTOC, bodyHTML := extractCoverAndTOCFromEditorHTML(doc.HTMLContent)
	bodyXML := convertEditorHTMLToOpenXML(bodyHTML, &embeddedImages)

	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	zw := zip.NewWriter(outFile)
	defer zw.Close()

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}

		switch f.Name {
		case "word/document.xml":
			updated, err := injectEditorBodyIntoMasterXML(string(raw), r, doc, bodyXML, editedCover, editedTOC)
			if err != nil {
				return err
			}
			raw = []byte(updated)
		case "word/_rels/document.xml.rels":
			if len(embeddedImages) > 0 {
				raw = []byte(appendImageRels(string(raw), embeddedImages))
			}
		case "[Content_Types].xml":
			if len(embeddedImages) > 0 {
				raw = []byte(ensureImageContentTypes(string(raw), embeddedImages))
			}
		case "word/footer1.xml", "word/footer2.xml", "word/footer3.xml":
			if strings.TrimSpace(doc.PageSettings.FooterText) != "" && strings.TrimSpace(doc.PageSettings.FooterText) != "Public" {
				raw = []byte(strings.ReplaceAll(string(raw), ">Public<", ">"+xmlEscape(doc.PageSettings.FooterText)+"<"))
			}
		}

		w, err := zw.Create(f.Name)
		if err != nil {
			return err
		}
		if _, err := w.Write(raw); err != nil {
			return err
		}
	}

	for _, img := range embeddedImages {
		w, err := zw.Create(img.ZipPath)
		if err != nil {
			return err
		}
		if _, err := w.Write(img.Bytes); err != nil {
			return err
		}
	}

	return nil
}

type editorCoverOverrides struct {
	Found         bool
	MonthYear     string
	Projects      string
	EmployeeName  string
	EmployeeCode  string
	AppraiserName string
}

type editorTOCEntry struct {
	IsProject bool
	Text      string
}

func extractCoverAndTOCFromEditorHTML(rawHTML string) (editorCoverOverrides, []editorTOCEntry, string) {
	rePB := regexp.MustCompile(`(?is)<div[^>]*data-page-break="true"[^>]*>.*?</div>`)
	parts := rePB.Split(rawHTML, -1)
	seps := rePB.FindAllString(rawHTML, -1)

	var cover editorCoverOverrides
	var toc []editorTOCEntry
	startIdx := 0

	stripTags := func(s string) string {
		re := regexp.MustCompile(`<[^>]+>`)
		return strings.TrimSpace(html.UnescapeString(re.ReplaceAllString(s, "")))
	}

	if len(parts) > 0 && (strings.Contains(parts[0], "Monthly Performance Report") || strings.Contains(parts[0], "Employee Name:")) {
		cover.Found = true
		rePara := regexp.MustCompile(`(?is)<(?:p|h1|h2|h3)[^>]*>(.*?)</(?:p|h1|h2|h3)>`)
		for _, m := range rePara.FindAllStringSubmatch(parts[0], -1) {
			txt := stripTags(m[1])
			low := strings.ToLower(txt)
			switch {
			case strings.HasPrefix(low, "projects"):
				if idx := strings.Index(txt, ":"); idx != -1 {
					cover.Projects = strings.TrimSpace(txt[idx+1:])
				}
			case strings.HasPrefix(low, "employee name"):
				if idx := strings.Index(txt, ":"); idx != -1 {
					cover.EmployeeName = strings.TrimSpace(txt[idx+1:])
				}
			case strings.HasPrefix(low, "employee code"):
				if idx := strings.Index(txt, ":"); idx != -1 {
					cover.EmployeeCode = strings.TrimSpace(txt[idx+1:])
				}
			case strings.HasPrefix(low, "appraiser name"):
				if idx := strings.Index(txt, ":"); idx != -1 {
					cover.AppraiserName = strings.TrimSpace(txt[idx+1:])
				}
			case !strings.Contains(low, "monthly performance report") && txt != "":
				cover.MonthYear = txt
			}
		}
		startIdx = 1
	}

	if startIdx < len(parts) && strings.Contains(parts[startIdx], "TABLE OF CONTENTS") {
		rePara := regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)
		for _, m := range rePara.FindAllStringSubmatch(parts[startIdx], -1) {
			txt := stripTags(m[1])
			if txt == "" {
				continue
			}
			if strings.HasPrefix(txt, "▪") {
				toc = append(toc, editorTOCEntry{IsProject: true, Text: strings.TrimSpace(strings.TrimPrefix(txt, "▪"))})
			} else if strings.HasPrefix(txt, "•") {
				toc = append(toc, editorTOCEntry{IsProject: false, Text: strings.TrimSpace(strings.TrimPrefix(txt, "•"))})
			} else {
				toc = append(toc, editorTOCEntry{IsProject: false, Text: txt})
			}
		}
		startIdx++
	}

	if startIdx == 0 {
		return cover, toc, rawHTML
	}

	var rest strings.Builder
	for i := startIdx; i < len(parts); i++ {
		if i > startIdx && i-1 < len(seps) {
			rest.WriteString(seps[i-1])
		}
		rest.WriteString(parts[i])
	}
	return cover, toc, rest.String()
}

func injectEditorBodyIntoMasterXML(docXML string, r *models.Report, doc *models.EditedDocument, bodyXML string, cov editorCoverOverrides, tocEntries []editorTOCEntry) (string, error) {
	bodyStartTagIdx := strings.Index(docXML, "<w:body>")
	if bodyStartTagIdx == -1 {
		bodyStartTagIdx = strings.Index(docXML, "<w:body ")
	}
	if bodyStartTagIdx == -1 {
		return "", fmt.Errorf("missing <w:body>")
	}
	bodyOpenEnd := strings.Index(docXML[bodyStartTagIdx:], ">")
	bodyContentStart := bodyStartTagIdx + bodyOpenEnd + 1

	sectPrIdx := strings.LastIndex(docXML, "<w:sectPr")
	if sectPrIdx == -1 || sectPrIdx < bodyContentStart {
		return "", fmt.Errorf("missing <w:sectPr>")
	}

	prefix := docXML[:bodyContentStart]
	bodyInner := docXML[bodyContentStart:sectPrIdx]
	origSectPr := docXML[sectPrIdx:]
	// Body section (Page 3+): remove titlePg and pgNumType w:start="1" so page 3+ continues numbering (3, 4, 5...)
	bodySectPr := strings.ReplaceAll(origSectPr, "<w:titlePg />", "")
	bodySectPr = strings.ReplaceAll(bodySectPr, "<w:titlePg/>", "")
	bodySectPr = regexp.MustCompile(`<w:pgNumType[^/]*/>`).ReplaceAllString(bodySectPr, "")
	suffix := updateSectPrPageSettings(bodySectPr, doc.PageSettings)

	c0, rest, err0 := extractTopLevelElement(bodyInner, "w:tbl")
	c1, rest, err1 := extractTopLevelElement(rest, "w:p")
	c2, rest, err2 := extractTopLevelElement(rest, "w:tbl")
	c3, _, err3 := extractTopLevelElement(rest, "w:sdt")

	var out strings.Builder
	out.WriteString(prefix)

	if err0 == nil && err1 == nil && err2 == nil && err3 == nil {
		projects := make([]models.ReportSection, len(r.Sections))
		copy(projects, r.Sections)
		sort.Slice(projects, func(i, j int) bool { return projects[i].Order < projects[j].Order })
		var names []string
		for _, p := range projects {
			if strings.TrimSpace(p.Project) != "" {
				names = append(names, strings.TrimSpace(p.Project))
			}
		}
		projectsLine := formatCoverProjectsLine(names)
		monthYear := strings.TrimSpace(r.CoverPage.MonthYear)
		if monthYear == "" {
			if t, pe := time.Parse("2006-01", r.Month); pe == nil {
				monthYear = t.Format("January 2006")
			}
		}
		empName := r.CoverPage.EmployeeName
		empCode := r.CoverPage.EmployeeCode
		appraiser := r.CoverPage.AppraiserName
		if cov.Found {
			if cov.Projects != "" {
				projectsLine = cov.Projects
			}
			if cov.MonthYear != "" {
				monthYear = cov.MonthYear
			}
			if cov.EmployeeName != "" {
				empName = cov.EmployeeName
			}
			if cov.EmployeeCode != "" {
				empCode = cov.EmployeeCode
			}
			if cov.AppraiserName != "" {
				appraiser = cov.AppraiserName
			}
		}
		c0 = updateCoverTbl1(c0, projectsLine, empName, empCode, appraiser)
		c2 = updateCoverTbl2(c2, monthYear)
		if len(tocEntries) > 0 {
			c3 = rebuildGoldenTOCFromEntries(c3, tocEntries)
		} else {
			c3 = rebuildGoldenTOC(c3, projects, r.Conclusion)
		}

		out.WriteString(c0)
		out.WriteString(c1)
		out.WriteString(c2)
		out.WriteString(c3)
		// Section break locks Cover (Page 1) and TOC (Page 2) to exact ERA 720/1152 margins so Page 1 never overflows
		out.WriteString(`<w:p><w:pPr><w:sectPr>` +
			`<w:headerReference w:type="default" r:id="rId11"/>` +
			`<w:footerReference w:type="even" r:id="rId12"/>` +
			`<w:footerReference w:type="default" r:id="rId13"/>` +
			`<w:footerReference w:type="first" r:id="rId14"/>` +
			`<w:pgSz w:w="12240" w:h="15840" w:code="1"/>` +
			`<w:pgMar w:top="720" w:right="1152" w:bottom="720" w:left="1152" w:header="0" w:footer="0" w:gutter="0"/>` +
			`<w:pgNumType w:start="1"/><w:titlePg/>` +
			`</w:sectPr></w:pPr></w:p>`)
	}

	out.WriteString(bodyXML)
	out.WriteString(suffix)
	return out.String(), nil
}

func rebuildGoldenTOCFromEntries(sdtXML string, entries []editorTOCEntry) string {
	contentStart := strings.Index(sdtXML, "<w:sdtContent>")
	contentEnd := strings.LastIndex(sdtXML, "</w:sdtContent>")
	if contentStart == -1 || contentEnd == -1 || contentEnd <= contentStart {
		return sdtXML
	}
	inner := sdtXML[contentStart+len("<w:sdtContent>") : contentEnd]
	firstP, _, err := extractTopLevelElement(inner, "w:p")
	if err != nil {
		return sdtXML
	}
	var toc strings.Builder
	toc.WriteString(firstP)
	for i, e := range entries {
		if e.IsProject {
			if i > 0 {
				toc.WriteString(`<w:p><w:pPr><w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:sz w:val="28"/><w:szCs w:val="28"/></w:rPr></w:pPr></w:p>`)
			}
			toc.WriteString(fmt.Sprintf(
				`<w:p><w:pPr><w:pStyle w:val="ListParagraph"/><w:numPr><w:ilvl w:val="0"/><w:numId w:val="5"/></w:numPr>`+
					`<w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:b/><w:bCs/><w:color w:val="1F1F1F"/><w:sz w:val="36"/><w:szCs w:val="36"/><w:lang w:val="en-IN"/></w:rPr></w:pPr>`+
					`<w:r><w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:b/><w:bCs/><w:color w:val="1F1F1F"/><w:sz w:val="36"/><w:szCs w:val="36"/><w:lang w:val="en-IN"/></w:rPr><w:t>%s</w:t></w:r></w:p>`,
				xmlEscape(e.Text),
			))
		} else {
			toc.WriteString(fmt.Sprintf(
				`<w:p><w:pPr><w:pStyle w:val="ListParagraph"/><w:numPr><w:ilvl w:val="0"/><w:numId w:val="6"/></w:numPr>`+
					`<w:ind w:left="1368"/><w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:color w:val="1F1F1F"/><w:sz w:val="28"/><w:szCs w:val="28"/><w:lang w:val="en-IN"/></w:rPr></w:pPr>`+
					`<w:r><w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:color w:val="1F1F1F"/><w:sz w:val="28"/><w:szCs w:val="28"/><w:lang w:val="en-IN"/></w:rPr><w:t>%s</w:t></w:r></w:p>`,
				xmlEscape(e.Text),
			))
		}
	}
	return sdtXML[:contentStart+len("<w:sdtContent>")] + toc.String() + sdtXML[contentEnd:]
}

func updateSectPrPageSettings(suffix string, ps models.EditorPageSettings) string {
	// Page size
	wTwips, hTwips := 12240, 15840 // US Letter
	if strings.EqualFold(ps.PageSize, "A4") {
		wTwips, hTwips = 11906, 16838
	}
	rePgSz := regexp.MustCompile(`<w:pgSz[^/]*/>`)
	suffix = rePgSz.ReplaceAllString(suffix, fmt.Sprintf(`<w:pgSz w:w="%d" w:h="%d"/>`, wTwips, hTwips))

	// Margins (mm -> twips: 1 mm = 56.692913 twips)
	mmToTwips := func(mm, def float64) int {
		if mm <= 0 {
			mm = def
		}
		return int(math.Round(mm * 56.692913))
	}
	top := mmToTwips(ps.MarginTopMM, 12.7)
	right := mmToTwips(ps.MarginRightMM, 20.32)
	bottom := mmToTwips(ps.MarginBottomMM, 12.7)
	left := mmToTwips(ps.MarginLeftMM, 20.32)
	rePgMar := regexp.MustCompile(`<w:pgMar[^/]*/>`)
	suffix = rePgMar.ReplaceAllString(suffix, fmt.Sprintf(`<w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="0" w:footer="0" w:gutter="0"/>`, top, right, bottom, left))
	return suffix
}

func appendImageRels(relsXML string, imgs []editorEmbeddedImage) string {
	idx := strings.LastIndex(relsXML, "</Relationships>")
	if idx == -1 {
		return relsXML
	}
	var add strings.Builder
	for _, img := range imgs {
		target := strings.TrimPrefix(img.ZipPath, "word/")
		add.WriteString(fmt.Sprintf(
			`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="%s"/>`,
			img.RelID, target,
		))
	}
	return relsXML[:idx] + add.String() + relsXML[idx:]
}

func ensureImageContentTypes(ctXML string, imgs []editorEmbeddedImage) string {
	idx := strings.LastIndex(ctXML, "</Types>")
	if idx == -1 {
		return ctXML
	}
	var add strings.Builder
	seen := map[string]bool{}
	for _, img := range imgs {
		ext := strings.ToLower(strings.TrimPrefix(img.Ext, "."))
		if ext == "" || seen[ext] {
			continue
		}
		seen[ext] = true
		if !strings.Contains(ctXML, `Extension="`+ext+`"`) {
			mime := "image/" + ext
			if ext == "jpg" {
				mime = "image/jpeg"
			}
			add.WriteString(fmt.Sprintf(`<Default Extension="%s" ContentType="%s"/>`, ext, mime))
		}
	}
	return ctXML[:idx] + add.String() + ctXML[idx:]
}

// ─── Lightweight HTML-to-OpenXML Converter ───────────────────────────────────

type htmlSpanStyle struct {
	Bold       bool
	Italic     bool
	Underline  bool
	FontFamily string
	FontSizePt float64
	ColorHex   string
}

func convertEditorHTMLToOpenXML(rawHTML string, images *[]editorEmbeddedImage) string {
	// Normalize self-closing tags and entities
	s := strings.ReplaceAll(rawHTML, "\r\n", "\n")

	// Tokenize into top-level / block & inline elements
	tokens := tokenizeHTML(s)
	var out strings.Builder

	type listState struct {
		ordered bool
		counter int
	}
	var listStack []listState
	var inTable bool
	var inRow bool
	var tableRows []string
	var curCells []string
	var cellParas []string

	styleStack := []htmlSpanStyle{{}}
	curStyle := func() htmlSpanStyle { return styleStack[len(styleStack)-1] }

	var paraOpen bool
	var paraRuns strings.Builder
	var paraAlign string
	var paraLineSpacing float64
	var paraBeforePt float64
	var paraAfterPt float64
	var paraIndentPx float64
	var paraHeading int // 0=normal, 1=h1, 2=h2, 3=h3
	var paraInBullet bool
	var paraBulletOrdered bool
	var paraBulletNum int

	flushPara := func() {
		if !paraOpen {
			return
		}
		paraOpen = false
		var pPr strings.Builder
		pPr.WriteString(`<w:pPr>`)
		if paraInBullet {
			if paraBulletOrdered {
				pPr.WriteString(`<w:numPr><w:ilvl w:val="0"/><w:numId w:val="9"/></w:numPr>`)
			} else {
				pPr.WriteString(`<w:numPr><w:ilvl w:val="0"/><w:numId w:val="9"/></w:numPr>`)
			}
		}
		if paraHeading > 0 {
			pPr.WriteString(`<w:keepNext/>`)
		}

		// Spacing
		beforeTw := int(math.Round(paraBeforePt * 20))
		afterTw := int(math.Round(paraAfterPt * 20))
		if afterTw == 0 && paraHeading == 0 {
			afterTw = 80
		}
		if paraHeading == 1 {
			beforeTw = 200
			afterTw = 120
		} else if paraHeading == 2 {
			beforeTw = 160
			afterTw = 80
		} else if paraHeading == 3 {
			beforeTw = 100
			afterTw = 40
		}
		lineAttr := ""
		if paraLineSpacing > 0 {
			lineAttr = fmt.Sprintf(` w:line="%d" w:lineRule="auto"`, int(math.Round(paraLineSpacing*240)))
		}
		pPr.WriteString(fmt.Sprintf(`<w:spacing w:before="%d" w:after="%d"%s/>`, beforeTw, afterTw, lineAttr))

		if paraIndentPx > 0 {
			twips := int(math.Round(paraIndentPx * 15))
			pPr.WriteString(fmt.Sprintf(`<w:ind w:left="%d"/>`, twips))
		}

		jc := "both"
		switch strings.ToLower(paraAlign) {
		case "left":
			jc = "left"
		case "center":
			jc = "center"
		case "right":
			jc = "right"
		case "justify":
			jc = "both"
		}
		pPr.WriteString(fmt.Sprintf(`<w:jc w:val="%s"/>`, jc))
		pPr.WriteString(`</w:pPr>`)

		pXML := `<w:p>` + pPr.String() + paraRuns.String() + `</w:p>`
		paraRuns.Reset()
		paraAlign = ""
		paraLineSpacing = 0
		paraBeforePt = 0
		paraAfterPt = 0
		paraIndentPx = 0
		paraHeading = 0
		paraInBullet = false

		if inTable {
			cellParas = append(cellParas, pXML)
		} else {
			out.WriteString(pXML)
		}
	}

	writeRun := func(text string, st htmlSpanStyle) {
		if text == "" {
			return
		}
		var rPr strings.Builder
		rPr.WriteString(`<w:rPr>`)
		font := st.FontFamily
		if font == "" {
			font = "Times New Roman"
		}
		rPr.WriteString(fmt.Sprintf(`<w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>`, xmlEscape(font), xmlEscape(font), xmlEscape(font)))
		if st.Bold {
			rPr.WriteString(`<w:b/><w:bCs/>`)
		}
		if st.Italic {
			rPr.WriteString(`<w:i/><w:iCs/>`)
		}
		if st.Underline {
			rPr.WriteString(`<w:u w:val="single"/>`)
		}
		if st.ColorHex != "" {
			rPr.WriteString(fmt.Sprintf(`<w:color w:val="%s"/>`, strings.TrimPrefix(st.ColorHex, "#")))
		}
		szPt := st.FontSizePt
		if szPt <= 0 {
			szPt = 14
		}
		szHalf := int(math.Round(szPt * 2))
		rPr.WriteString(fmt.Sprintf(`<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, szHalf, szHalf))
		rPr.WriteString(`<w:lang w:val="en-IN"/></w:rPr>`)

		paraRuns.WriteString(`<w:r>` + rPr.String() + `<w:t xml:space="preserve">` + xmlEscape(text) + `</w:t></w:r>`)
	}

	for _, tok := range tokens {
		if !tok.isTag {
			txt := html.UnescapeString(tok.text)
			if !paraOpen {
				if strings.TrimSpace(txt) == "" {
					continue
				}
				paraOpen = true
			}
			writeRun(txt, curStyle())
			continue
		}

		tag := strings.ToLower(tok.tagName)
		if tok.isClose {
			switch tag {
			case "p", "h1", "h2", "h3", "h4":
				flushPara()
				if len(styleStack) > 1 {
					styleStack = styleStack[:len(styleStack)-1]
				}
			case "ul", "ol":
				flushPara()
				if len(listStack) > 0 {
					listStack = listStack[:len(listStack)-1]
				}
			case "li":
				flushPara()
			case "strong", "b", "em", "i", "u", "span":
				if len(styleStack) > 1 {
					styleStack = styleStack[:len(styleStack)-1]
				}
			case "td", "th":
				flushPara()
				if len(cellParas) == 0 {
					cellParas = append(cellParas, `<w:p/>`)
				}
				curCells = append(curCells, `<w:tc><w:tcPr><w:tcBorders><w:top w:val="single" w:sz="4" w:color="CBD5E1"/><w:left w:val="single" w:sz="4" w:color="CBD5E1"/><w:bottom w:val="single" w:sz="4" w:color="CBD5E1"/><w:right w:val="single" w:sz="4" w:color="CBD5E1"/></w:tcBorders></w:tcPr>`+strings.Join(cellParas, "")+`</w:tc>`)
				cellParas = nil
			case "tr":
				if inRow {
					tableRows = append(tableRows, `<w:tr>`+strings.Join(curCells, "")+`</w:tr>`)
					curCells = nil
					inRow = false
				}
			case "table":
				if inTable {
					out.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="9936" w:type="dxa"/><w:tblBorders><w:top w:val="single" w:sz="4" w:color="CBD5E1"/><w:left w:val="single" w:sz="4" w:color="CBD5E1"/><w:bottom w:val="single" w:sz="4" w:color="CBD5E1"/><w:right w:val="single" w:sz="4" w:color="CBD5E1"/><w:insideH w:val="single" w:sz="4" w:color="CBD5E1"/><w:insideV w:val="single" w:sz="4" w:color="CBD5E1"/></w:tblBorders></w:tblPr>` + strings.Join(tableRows, "") + `</w:tbl>`)
					tableRows = nil
					inTable = false
				}
			}
			continue
		}

		// Opening or self-closing tag
		switch tag {
		case "div":
			if tok.attrs["data-page-break"] == "true" || strings.Contains(tok.attrs["class"], "page-break") {
				flushPara()
				out.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
			}
		case "hr":
			flushPara()
			out.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		case "br":
			if paraOpen {
				paraRuns.WriteString(`<w:r><w:br/></w:r>`)
			}
		case "ul":
			flushPara()
			listStack = append(listStack, listState{ordered: false})
		case "ol":
			flushPara()
			listStack = append(listStack, listState{ordered: true})
		case "li":
			flushPara()
			if len(listStack) > 0 {
				listStack[len(listStack)-1].counter++
			}
		case "p", "h1", "h2", "h3", "h4":
			flushPara()
			paraOpen = true
			st := curStyle()
			if tag == "h1" {
				paraHeading = 1
				st.Bold = true
				if st.FontFamily == "" {
					st.FontFamily = "Century Gothic"
				}
				if st.FontSizePt <= 0 {
					st.FontSizePt = 23
				}
			} else if tag == "h2" {
				paraHeading = 2
				st.Bold = true
				if st.FontSizePt <= 0 {
					st.FontSizePt = 16
				}
			} else if tag == "h3" || tag == "h4" {
				paraHeading = 3
				st.Bold = true
				if st.FontSizePt <= 0 {
					st.FontSizePt = 14
				}
			}
			applyInlineStyle(tok.attrs["style"], &st, &paraAlign, &paraLineSpacing, &paraBeforePt, &paraAfterPt, &paraIndentPx)
			styleStack = append(styleStack, st)
			if len(listStack) > 0 {
				top := listStack[len(listStack)-1]
				paraInBullet = true
				paraBulletOrdered = top.ordered
				paraBulletNum = top.counter
				if paraBulletOrdered {
					// Emit explicit number prefix if ordered list
					writeRun(fmt.Sprintf("%d. ", paraBulletNum), st)
					paraInBullet = false
					paraIndentPx += 24
				}
			}
		case "strong", "b":
			st := curStyle()
			st.Bold = true
			styleStack = append(styleStack, st)
		case "em", "i":
			st := curStyle()
			st.Italic = true
			styleStack = append(styleStack, st)
		case "u":
			st := curStyle()
			st.Underline = true
			styleStack = append(styleStack, st)
		case "span":
			st := curStyle()
			var dummyA string
			var dummyF float64
			applyInlineStyle(tok.attrs["style"], &st, &dummyA, &dummyF, &dummyF, &dummyF, &dummyF)
			styleStack = append(styleStack, st)
		case "table":
			flushPara()
			inTable = true
			tableRows = nil
		case "tr":
			inRow = true
			curCells = nil
		case "td", "th":
			cellParas = nil
		case "img":
			src := tok.attrs["src"]
			if strings.HasPrefix(src, "data:image/") {
				if imgXML, ok := buildImageRunFromDataURI(src, tok.attrs["style"], tok.attrs["width"], images); ok {
					if !paraOpen {
						paraOpen = true
					}
					paraRuns.WriteString(imgXML)
				}
			}
		}
	}
	flushPara()

	if out.Len() == 0 {
		return `<w:p><w:r><w:t></w:t></w:r></w:p>`
	}
	return out.String()
}

func applyInlineStyle(style string, st *htmlSpanStyle, align *string, lineSp, beforePt, afterPt, indentPx *float64) {
	if style == "" {
		return
	}
	for _, decl := range strings.Split(style, ";") {
		parts := strings.SplitN(decl, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.TrimSpace(parts[1])
		switch k {
		case "font-family":
			f := strings.Split(v, ",")[0]
			f = strings.Trim(f, ` "'`)
			if f != "" {
				st.FontFamily = f
			}
		case "font-size":
			if pt, ok := parseCSSPt(v); ok {
				st.FontSizePt = pt
			}
		case "color":
			if hex := parseCSSColorHex(v); hex != "" {
				st.ColorHex = hex
			}
		case "font-weight":
			if strings.Contains(strings.ToLower(v), "bold") || v == "700" || v == "600" {
				st.Bold = true
			}
		case "text-align":
			*align = v
		case "line-height":
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f > 0 {
				*lineSp = f
			}
		case "margin-top":
			if pt, ok := parseCSSPt(v); ok {
				*beforePt = pt
			}
		case "margin-bottom":
			if pt, ok := parseCSSPt(v); ok {
				*afterPt = pt
			}
		case "margin-left", "padding-left":
			vClean := strings.TrimSuffix(strings.ToLower(v), "px")
			if px, err := strconv.ParseFloat(strings.TrimSpace(vClean), 64); err == nil && px > 0 {
				*indentPx = px
			}
		}
	}
}

func parseCSSPt(v string) (float64, bool) {
	lv := strings.ToLower(strings.TrimSpace(v))
	if strings.HasSuffix(lv, "pt") {
		if f, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(lv, "pt")), 64); err == nil {
			return f, true
		}
	} else if strings.HasSuffix(lv, "px") {
		if f, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(lv, "px")), 64); err == nil {
			return f * 0.75, true
		}
	}
	return 0, false
}

func parseCSSColorHex(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "#") && (len(v) == 7 || len(v) == 4) {
		h := strings.TrimPrefix(v, "#")
		if len(h) == 3 {
			return fmt.Sprintf("%c%c%c%c%c%c", h[0], h[0], h[1], h[1], h[2], h[2])
		}
		return strings.ToUpper(h)
	}
	reRGB := regexp.MustCompile(`rgb\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)`)
	if m := reRGB.FindStringSubmatch(strings.ToLower(v)); len(m) == 4 {
		r, _ := strconv.Atoi(m[1])
		g, _ := strconv.Atoi(m[2])
		b, _ := strconv.Atoi(m[3])
		return fmt.Sprintf("%02X%02X%02X", r, g, b)
	}
	return ""
}

func buildImageRunFromDataURI(dataURI, style, widthAttr string, images *[]editorEmbeddedImage) (string, bool) {
	comma := strings.Index(dataURI, ",")
	if comma == -1 {
		return "", false
	}
	meta := dataURI[:comma]
	b64 := dataURI[comma+1:]
	rawBytes, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(rawBytes) == 0 {
		return "", false
	}
	ext := "png"
	if strings.Contains(meta, "jpeg") || strings.Contains(meta, "jpg") {
		ext = "jpg"
	} else if strings.Contains(meta, "gif") {
		ext = "gif"
	}

	idx := len(*images) + 1
	relID := fmt.Sprintf("rIdEd%d", idx)
	zipPath := fmt.Sprintf("word/media/editor_img_%d.%s", idx, ext)
	docPrID := 500 + idx
	*images = append(*images, editorEmbeddedImage{
		RelID:   relID,
		ZipPath: zipPath,
		Ext:     ext,
		Bytes:   rawBytes,
		DocPrID: docPrID,
	})

	widthPx := 420.0
	if w, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(widthAttr), "px"), 64); err == nil && w > 20 {
		widthPx = w
	}
	reW := regexp.MustCompile(`width\s*:\s*(\d+(?:\.\d+)?)px`)
	if m := reW.FindStringSubmatch(style); len(m) == 2 {
		if w, err := strconv.ParseFloat(m[1], 64); err == nil && w > 20 {
			widthPx = w
		}
	}
	if widthPx > 600 {
		widthPx = 600
	}
	heightPx := widthPx * 0.6
	cx := int64(widthPx * 9525)
	cy := int64(heightPx * 9525)

	xmlStr := fmt.Sprintf(
		`<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="EditorImage%d"/><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr><pic:cNvPr id="%d" name="EditorImage%d"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`,
		cx, cy, docPrID, idx, docPrID, idx, relID, cx, cy,
	)
	return xmlStr, true
}

type htmlToken struct {
	isTag   bool
	isClose bool
	tagName string
	attrs   map[string]string
	text    string
}

func tokenizeHTML(s string) []htmlToken {
	var out []htmlToken
	i := 0
	for i < len(s) {
		lt := strings.IndexByte(s[i:], '<')
		if lt == -1 {
			out = append(out, htmlToken{text: s[i:]})
			break
		}
		if lt > 0 {
			out = append(out, htmlToken{text: s[i : i+lt]})
		}
		gt := strings.IndexByte(s[i+lt:], '>')
		if gt == -1 {
			out = append(out, htmlToken{text: s[i+lt:]})
			break
		}
		rawTag := strings.TrimSpace(s[i+lt+1 : i+lt+gt])
		i = i + lt + gt + 1
		if rawTag == "" || strings.HasPrefix(rawTag, "!") {
			continue
		}
		isClose := false
		if strings.HasPrefix(rawTag, "/") {
			isClose = true
			rawTag = strings.TrimSpace(rawTag[1:])
		}
		rawTag = strings.TrimSuffix(rawTag, "/")
		fields := strings.Fields(rawTag)
		if len(fields) == 0 {
			continue
		}
		tagName := fields[0]
		attrs := map[string]string{}
		if !isClose && len(rawTag) > len(tagName) {
			attrStr := rawTag[len(tagName):]
			reAttr := regexp.MustCompile(`([a-zA-Z0-9_-]+)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
			for _, m := range reAttr.FindAllStringSubmatch(attrStr, -1) {
				val := m[2]
				if val == "" {
					val = m[3]
				}
				attrs[strings.ToLower(m[1])] = val
			}
		}
		out = append(out, htmlToken{
			isTag:   true,
			isClose: isClose,
			tagName: tagName,
			attrs:   attrs,
		})
	}
	return out
}

func renderEditorHTMLToPDF(doc *models.EditedDocument, pdfPath string) error {
	browserExe := findHeadlessBrowser()
	if browserExe == "" {
		return fmt.Errorf("no headless browser found")
	}
	ps := doc.PageSettings
	pageSizeCSS := "8.5in 11in"
	if strings.EqualFold(ps.PageSize, "A4") {
		pageSizeCSS = "210mm 297mm"
	}
	fullHTML := fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"><style>
@page { size: %s; margin: %.2fmm %.2fmm %.2fmm %.2fmm; }
body { font-family: "Times New Roman", serif; font-size: 14pt; color: #1f2937; margin: 0; }
[data-page-break="true"] { page-break-before: always; break-before: page; height: 0; }
h1, h2, h3 { break-after: avoid; page-break-after: avoid; }
table { width: 100%%; border-collapse: collapse; }
td, th { border: 1px solid #cbd5e1; padding: 6px 8px; }
</style></head><body>%s</body></html>`,
		pageSizeCSS, ps.MarginTopMM, ps.MarginRightMM, ps.MarginBottomMM, ps.MarginLeftMM, doc.HTMLContent)

	tmpHTML := filepath.Join(filepath.Dir(pdfPath), "editor_fallback.html")
	if err := os.WriteFile(tmpHTML, []byte(fullHTML), 0644); err != nil {
		return err
	}
	absHTML, _ := filepath.Abs(tmpHTML)
	absPDF, _ := filepath.Abs(pdfPath)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, browserExe, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-pdf-header-footer", "--print-to-pdf="+absPDF, "file:///"+filepath.ToSlash(absHTML))
	_, err := cmd.CombinedOutput()
	return err
}
