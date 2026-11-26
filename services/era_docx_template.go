package services

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"performancereport/models"
	"performancereport/storage"
)

// ERAMasterTemplateBytes holds the embedded assets/ERA_Master_Template.docx bytes when built with go:embed.
var ERAMasterTemplateBytes []byte

func escapeXML(s string) string {
	return xmlEscape(s)
}

var reLeadingProjectNum = regexp.MustCompile(`^\s*\d+[\.\)\-:]\s*`)

func formatCoverProjectsLine(names []string) string {
	var cleaned []string
	seen := map[string]bool{}
	for _, raw := range names {
		for _, part := range splitProjectList(raw) {
			p := strings.TrimSpace(reLeadingProjectNum.ReplaceAllString(strings.TrimSpace(part), ""))
			if p == "" {
				continue
			}
			low := strings.ToLower(p)
			if !seen[low] {
				seen[low] = true
				cleaned = append(cleaned, p)
			}
		}
	}
	if len(cleaned) == 0 {
		return "Monthly Tasks"
	}
	if len(cleaned) == 1 {
		return cleaned[0]
	}
	return strings.Join(cleaned, ", ")
}

func loadERAMasterDocxBytes() ([]byte, error) {
	if len(ERAMasterTemplateBytes) > 0 {
		return ERAMasterTemplateBytes, nil
	}
	candidates := []string{
		filepath.Join("assets", "ERA_Master_Template.docx"),
		filepath.Join("..", "assets", "ERA_Master_Template.docx"),
		filepath.Join(storage.DataPath(), "templates", "era", "source.docx"),
	}
	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			return data, nil
		}
	}
	return nil, fmt.Errorf("ERA_Master_Template.docx not found")
}

// generateDocxFromMasterTemplate generates a DOCX by cloning the master ERA .docx archive
// (preserving all images, headers, footers, styles, numbering, margins, and US Letter page setup)
// and replacing cover placeholders, golden TOC (without page numbers), and body sections.
func generateDocxFromMasterTemplate(r *models.Report, tmpl *models.Template, outputPath string) error {
	masterBytes, err := loadERAMasterDocxBytes()
	if err != nil {
		return err
	}

	zr, err := zip.NewReader(bytes.NewReader(masterBytes), int64(len(masterBytes)))
	if err != nil {
		return fmt.Errorf("open master docx zip: %w", err)
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output docx: %w", err)
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

		if f.Name == "word/document.xml" {
			updated, err := buildERADocumentXML(string(raw), r, tmpl)
			if err != nil {
				return fmt.Errorf("build document.xml: %w", err)
			}
			raw = []byte(updated)
		}

		w, err := zw.Create(f.Name)
		if err != nil {
			return err
		}
		if _, err := w.Write(raw); err != nil {
			return err
		}
	}

	return nil
}

func buildERADocumentXML(docXML string, r *models.Report, tmpl *models.Template) (string, error) {
	bodyStartTagIdx := strings.Index(docXML, "<w:body>")
	if bodyStartTagIdx == -1 {
		bodyStartTagIdx = strings.Index(docXML, "<w:body ")
	}
	if bodyStartTagIdx == -1 {
		return "", fmt.Errorf("missing <w:body> in master document.xml")
	}
	bodyOpenEnd := strings.Index(docXML[bodyStartTagIdx:], ">")
	if bodyOpenEnd == -1 {
		return "", fmt.Errorf("malformed <w:body> tag")
	}
	bodyContentStart := bodyStartTagIdx + bodyOpenEnd + 1

	sectPrIdx := strings.LastIndex(docXML, "<w:sectPr")
	if sectPrIdx == -1 || sectPrIdx < bodyContentStart {
		return "", fmt.Errorf("missing <w:sectPr> in master document.xml")
	}
	bodyEndIdx := strings.LastIndex(docXML, "</w:body>")
	if bodyEndIdx == -1 || bodyEndIdx < sectPrIdx {
		return "", fmt.Errorf("missing </w:body> in master document.xml")
	}

	prefix := docXML[:bodyContentStart]
	bodyInner := docXML[bodyContentStart:sectPrIdx]
	suffix := docXML[sectPrIdx:] // includes <w:sectPr ...>...</w:sectPr></w:body></w:document>

	// Extract children 0, 1, 2, 3 from bodyInner:
	// Child 0: <w:tbl>...</w:tbl> (Cover Projects + Employee info)
	// Child 1: <w:p ...>...</w:p> (Cover backdrop images & yellow rectangle)
	// Child 2: <w:tbl>...</w:tbl> (Cover purple banner + Monthly Performance Report + Month Year)
	// Child 3: <w:sdt>...</w:sdt> (Golden Yellow TOC)
	c0, rest, err := extractTopLevelElement(bodyInner, "w:tbl")
	if err != nil {
		return "", fmt.Errorf("extract cover tbl1: %w", err)
	}
	c1, rest, err := extractTopLevelElement(rest, "w:p")
	if err != nil {
		return "", fmt.Errorf("extract cover backdrop p: %w", err)
	}
	c2, rest, err := extractTopLevelElement(rest, "w:tbl")
	if err != nil {
		return "", fmt.Errorf("extract cover tbl2: %w", err)
	}
	c3, _, err := extractTopLevelElement(rest, "w:sdt")
	if err != nil {
		return "", fmt.Errorf("extract toc sdt: %w", err)
	}

	projects := make([]models.ReportSection, len(r.Sections))
	copy(projects, r.Sections)
	sort.Slice(projects, func(i, j int) bool { return projects[i].Order < projects[j].Order })

	projectNames := make([]string, 0, len(projects))
	for _, p := range projects {
		if strings.TrimSpace(p.Project) != "" {
			projectNames = append(projectNames, strings.TrimSpace(p.Project))
		}
	}
	if len(projectNames) == 0 && len(r.CoverPage.Projects) > 0 {
		projectNames = append(projectNames, r.CoverPage.Projects...)
	}
	projectsLine := formatCoverProjectsLine(projectNames)

	// 1. Update Cover Page tbl 1 (Projects + Employee Name/Code/Appraiser Name)
	c0 = updateCoverTbl1(c0, projectsLine, r.CoverPage.EmployeeName, r.CoverPage.EmployeeCode, r.CoverPage.AppraiserName)

	// 2. Update Cover Page tbl 2 (Month Year)
	monthYear := strings.TrimSpace(r.CoverPage.MonthYear)
	if monthYear == "" {
		if t, parseErr := time.Parse("2006-01", r.Month); parseErr == nil {
			monthYear = t.Format("January 2006")
		} else {
			monthYear = strings.TrimSpace(r.CoverPage.Month + " " + r.CoverPage.Year)
		}
	}
	c2 = updateCoverTbl2(c2, monthYear)

	// 3. Rebuild Golden Yellow TOC <w:sdt> without page numbers or dotted leaders
	c3 = rebuildGoldenTOC(c3, projects, r.Conclusion)

	// 4. Build Body Content (Projects, Tasks, Subheadings, Bullets, Descriptions, Conclusion)
	bodyContent := buildERABodyContent(r, tmpl, projects)

	var out strings.Builder
	out.Grow(len(prefix) + len(c0) + len(c1) + len(c2) + len(c3) + len(bodyContent) + len(suffix) + 1024)
	out.WriteString(prefix)
	out.WriteString(c0)
	out.WriteString(c1)
	out.WriteString(c2)
	out.WriteString(c3)
	// Page break after TOC so Page 3 begins cleanly with Project 1
	out.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
	out.WriteString(bodyContent)
	out.WriteString(suffix)
	return out.String(), nil
}

// extractTopLevelElement extracts the first balanced <tagName ...>...</tagName> from s
// and returns (element, remainder, error).
func extractTopLevelElement(s, tagName string) (string, string, error) {
	openPrefix := "<" + tagName
	start := strings.Index(s, openPrefix)
	if start == -1 {
		return "", "", fmt.Errorf("tag <%s> not found", tagName)
	}
	// Ensure exact tag boundary
	afterName := start + len(openPrefix)
	if afterName < len(s) && s[afterName] != '>' && s[afterName] != ' ' && s[afterName] != '\t' && s[afterName] != '\r' && s[afterName] != '\n' {
		return "", "", fmt.Errorf("tag <%s> boundary mismatch", tagName)
	}

	closeTag := "</" + tagName + ">"
	depth := 0
	pos := start
	for pos < len(s) {
		nextOpen := strings.Index(s[pos:], openPrefix)
		nextClose := strings.Index(s[pos:], closeTag)
		if nextClose == -1 {
			return "", "", fmt.Errorf("unclosed <%s>", tagName)
		}
		if nextOpen != -1 && nextOpen < nextClose {
			absOpen := pos + nextOpen
			// Check boundary and self-closing
			tagEnd := strings.IndexByte(s[absOpen:], '>')
			if tagEnd == -1 {
				return "", "", fmt.Errorf("malformed <%s>", tagName)
			}
			chAfter := s[absOpen+len(openPrefix)]
			if chAfter == '>' || chAfter == ' ' || chAfter == '\t' || chAfter == '\r' || chAfter == '\n' {
				if s[absOpen+tagEnd-1] != '/' {
					depth++
				}
			}
			pos = absOpen + tagEnd + 1
		} else {
			absClose := pos + nextClose
			depth--
			pos = absClose + len(closeTag)
			if depth == 0 {
				return s[start:pos], s[pos:], nil
			}
		}
	}
	return "", "", fmt.Errorf("unclosed <%s>", tagName)
}

func updateCoverTbl1(tblXML, projectsLine, empName, empCode, appraiser string) string {
	// Text Box 6 holds "Projects : ..." (both mc:Choice and mc:Fallback copies)
	projContent := `<w:txbxContent>` +
		`<w:p><w:pPr><w:pStyle w:val="Subtitle"/><w:rPr><w:sz w:val="28"/><w:szCs w:val="28"/><w:lang w:val="en-IN"/></w:rPr></w:pPr>` +
		`<w:r><w:rPr><w:sz w:val="28"/><w:szCs w:val="28"/><w:lang w:val="en-IN"/></w:rPr><w:t xml:space="preserve">Projects :  ` + escapeXML(projectsLine) + `</w:t></w:r></w:p>` +
		`</w:txbxContent>`

	// Text Box 7 holds Employee Name / Employee Code / Appraiser Name
	empContent := `<w:txbxContent>` +
		coverFieldPara("Employee Name: ", empName) +
		coverFieldPara("Employee Code: ", empCode) +
		coverFieldPara("Appraiser Name: ", appraiser) +
		`</w:txbxContent>`

	// Replace every <w:txbxContent>...</w:txbxContent> in order:
	// First two belong to Text Box 6 (Choice + Fallback), next two belong to Text Box 7 (Choice + Fallback).
	re := regexp.MustCompile(`(?s)<w:txbxContent>.*?</w:txbxContent>`)
	idx := 0
	return re.ReplaceAllStringFunc(tblXML, func(m string) string {
		idx++
		if strings.Contains(m, "Project") || idx <= 2 {
			return projContent
		}
		return empContent
	})
}

func coverFieldPara(label, value string) string {
	rPr := `<w:rPr><w:rFonts w:ascii="Avenir Next LT Pro Light" w:hAnsi="Avenir Next LT Pro Light"/><w:b/><w:bCs/><w:sz w:val="28"/><w:szCs w:val="28"/><w:lang w:val="en-IN"/></w:rPr>`
	return `<w:p><w:pPr>` + rPr + `</w:pPr>` +
		`<w:r>` + rPr + `<w:t xml:space="preserve">` + escapeXML(label+" "+strings.TrimSpace(value)) + `</w:t></w:r>` +
		`</w:p>`
}

func updateCoverTbl2(tblXML, monthYear string) string {
	// Replace "October 2026" (the month/year paragraph in Cover tbl 2)
	re := regexp.MustCompile(`(<w:t[^>]*>)\s*October 2026\s*(</w:t>)`)
	if re.MatchString(tblXML) {
		return re.ReplaceAllString(tblXML, `${1}`+escapeXML(monthYear)+`${2}`)
	}
	// Fallback: replace any "<Month> <YYYY>" inside tbl2
	reAny := regexp.MustCompile(`(<w:t[^>]*>)\s*[A-Z][a-z]+\s+\d{4}\s*(</w:t>)`)
	return reAny.ReplaceAllString(tblXML, `${1}`+escapeXML(monthYear)+`${2}`)
}

func rebuildGoldenTOC(sdtXML string, projects []models.ReportSection, conclusion string) string {
	contentStart := strings.Index(sdtXML, "<w:sdtContent>")
	contentEnd := strings.LastIndex(sdtXML, "</w:sdtContent>")
	if contentStart == -1 || contentEnd == -1 || contentEnd <= contentStart {
		return sdtXML
	}
	inner := sdtXML[contentStart+len("<w:sdtContent>") : contentEnd]
	// The first <w:p> inside <w:sdtContent> holds Rectangle 31 (golden background) + "TABLE OF CONTENTS"
	firstP, _, err := extractTopLevelElement(inner, "w:p")
	if err != nil {
		return sdtXML
	}

	var toc strings.Builder
	toc.WriteString(firstP)

	for pi, proj := range projects {
		pName := strings.TrimSpace(proj.Project)
		if pName == "" {
			continue
		}
		pAnchor := fmt.Sprintf("proj_%d", pi+1)
		// Project entry (square bullet numId="5", 18pt Bold Times New Roman, hyperlink, NO page number)
		toc.WriteString(fmt.Sprintf(
			`<w:p><w:pPr><w:pStyle w:val="ListParagraph"/><w:numPr><w:ilvl w:val="0"/><w:numId w:val="5"/></w:numPr>`+
				`<w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:b/><w:bCs/><w:color w:val="1F1F1F"/><w:sz w:val="36"/><w:szCs w:val="36"/><w:lang w:val="en-IN"/></w:rPr></w:pPr>`+
				`<w:hyperlink w:anchor="%s" w:history="1"><w:r><w:rPr><w:rStyle w:val="Hyperlink"/><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:b/><w:bCs/><w:color w:val="1F1F1F"/><w:sz w:val="36"/><w:szCs w:val="36"/><w:u w:val="none"/><w:lang w:val="en-IN"/></w:rPr><w:t>%s</w:t></w:r></w:hyperlink></w:p>`,
			pAnchor, escapeXML(pName),
		))

		for ti, task := range proj.Tasks {
			tTitle := strings.TrimSpace(task.Title)
			if tTitle == "" {
				continue
			}
			tAnchor := fmt.Sprintf("task_%d_%d", pi+1, ti+1)
			// Task entry (round bullet numId="6", indented 1368 twips, 14pt Times New Roman, hyperlink, NO page number)
			toc.WriteString(fmt.Sprintf(
				`<w:p><w:pPr><w:pStyle w:val="ListParagraph"/><w:numPr><w:ilvl w:val="0"/><w:numId w:val="6"/></w:numPr>`+
					`<w:ind w:left="1368"/><w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:color w:val="1F1F1F"/><w:sz w:val="28"/><w:szCs w:val="28"/><w:lang w:val="en-IN"/></w:rPr></w:pPr>`+
					`<w:hyperlink w:anchor="%s" w:history="1"><w:r><w:rPr><w:rStyle w:val="Hyperlink"/><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:color w:val="1F1F1F"/><w:sz w:val="28"/><w:szCs w:val="28"/><w:u w:val="none"/><w:lang w:val="en-IN"/></w:rPr><w:t>%s</w:t></w:r></w:hyperlink></w:p>`,
				tAnchor, escapeXML(tTitle),
			))
		}

		// Spacer between project groups
		toc.WriteString(`<w:p><w:pPr><w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:sz w:val="28"/><w:szCs w:val="28"/></w:rPr></w:pPr></w:p>`)
	}

	if strings.TrimSpace(rConclusionTrim(conclusion)) != "" {
		toc.WriteString(
			`<w:p><w:pPr><w:pStyle w:val="ListParagraph"/><w:numPr><w:ilvl w:val="0"/><w:numId w:val="5"/></w:numPr>` +
				`<w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:b/><w:bCs/><w:color w:val="1F1F1F"/><w:sz w:val="36"/><w:szCs w:val="36"/><w:lang w:val="en-IN"/></w:rPr></w:pPr>` +
				`<w:hyperlink w:anchor="Conclusion" w:history="1"><w:r><w:rPr><w:rStyle w:val="Hyperlink"/><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:b/><w:bCs/><w:color w:val="1F1F1F"/><w:sz w:val="36"/><w:szCs w:val="36"/><w:u w:val="none"/><w:lang w:val="en-IN"/></w:rPr><w:t>Conclusion</w:t></w:r></w:hyperlink></w:p>`,
		)
	}

	return sdtXML[:contentStart+len("<w:sdtContent>")] + toc.String() + sdtXML[contentEnd:]
}

func rConclusionTrim(s string) string {
	return strings.TrimSpace(s)
}

func buildERABodyContent(r *models.Report, tmpl *models.Template, projects []models.ReportSection) string {
	isCustom := strings.EqualFold(r.FormattingMode, "custom") || (tmpl != nil && strings.EqualFold(tmpl.FormattingMode, "custom"))

	// Default ERA "Match Original Template" sizes (in half-points):
	// Project = 26pt (52), Task Title = 16pt (32), Subheading = 15pt (30), Body/Bullets = 14pt (28)
	projSz := 52
	projFontXML := `<w:rFonts w:asciiTheme="majorHAnsi" w:hAnsiTheme="majorHAnsi"/>`
	projColorXML := ``
	taskSz := 32
	subSz := 30
	bodySz := 28

	if isCustom {
		// Custom Formatting specification:
		// Project Name: Century Gothic, 23 pt (46 half-pt), Bold
		// Task Title: Times New Roman, 16 pt (32 half-pt), Bold
		// Subheadings: Times New Roman, 14 pt (28 half-pt), Bold
		// Description: Times New Roman, 14 pt (28 half-pt), Regular
		projSz = 46
		projFontXML = `<w:rFonts w:ascii="Century Gothic" w:hAnsi="Century Gothic" w:cs="Century Gothic"/>`
		projColorXML = `<w:color w:val="262262"/>`
		subSz = 28
		if tmpl != nil {
			if tmpl.Typography.ProjectName.FontSizePt > 0 {
				projSz = int(tmpl.Typography.ProjectName.FontSizePt * 2)
			}
			if strings.TrimSpace(tmpl.Typography.ProjectName.FontFamily) != "" {
				f := xmlEscape(strings.TrimSpace(tmpl.Typography.ProjectName.FontFamily))
				projFontXML = fmt.Sprintf(`<w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>`, f, f, f)
			}
			if tmpl.Typography.TaskTitle.FontSizePt > 0 {
				taskSz = int(tmpl.Typography.TaskTitle.FontSizePt * 2)
			}
			if tmpl.Typography.Subheading.FontSizePt > 0 {
				subSz = int(tmpl.Typography.Subheading.FontSizePt * 2)
			}
			if tmpl.Typography.TaskDescription.FontSizePt > 0 {
				bodySz = int(tmpl.Typography.TaskDescription.FontSizePt * 2)
			}
		}
	}

	var b strings.Builder
	bookmarkID := 100

	writeProjectHeading := func(title, anchor string) {
		bm := strconv.Itoa(bookmarkID)
		bookmarkID++
		rPr := fmt.Sprintf(`%s<w:b/><w:bCs/>%s<w:sz w:val="%d"/><w:szCs w:val="%d"/><w:lang w:val="en-IN"/>`,
			projFontXML, projColorXML, projSz, projSz)
		b.WriteString(fmt.Sprintf(
			`<w:p><w:pPr><w:keepNext/><w:spacing w:before="200" w:after="120"/><w:jc w:val="both"/><w:rPr>%s</w:rPr></w:pPr>`+
				`<w:bookmarkStart w:id="%s" w:name="%s"/><w:r><w:rPr>%s</w:rPr><w:t>%s</w:t></w:r><w:bookmarkEnd w:id="%s"/></w:p>`,
			rPr, bm, escapeXML(anchor), rPr, escapeXML(title), bm,
		))
	}

	writeTaskTitle := func(title, anchor string) {
		bm := strconv.Itoa(bookmarkID)
		bookmarkID++
		rPr := fmt.Sprintf(`<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:b/><w:bCs/><w:sz w:val="%d"/><w:szCs w:val="%d"/><w:lang w:val="en-IN"/>`,
			taskSz, taskSz)
		b.WriteString(fmt.Sprintf(
			`<w:p><w:pPr><w:pStyle w:val="ListParagraph"/><w:numPr><w:ilvl w:val="0"/><w:numId w:val="8"/></w:numPr>`+
				`<w:keepNext/><w:spacing w:before="160" w:after="80"/><w:jc w:val="both"/><w:rPr>%s</w:rPr></w:pPr>`+
				`<w:bookmarkStart w:id="%s" w:name="%s"/><w:r><w:rPr>%s</w:rPr><w:t>%s</w:t></w:r><w:bookmarkEnd w:id="%s"/></w:p>`,
			rPr, bm, escapeXML(anchor), rPr, escapeXML(title), bm,
		))
	}

	writeSubheading := func(heading string) {
		rPr := fmt.Sprintf(`<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:b/><w:bCs/><w:sz w:val="%d"/><w:szCs w:val="%d"/><w:lang w:val="en-IN"/>`,
			subSz, subSz)
		b.WriteString(fmt.Sprintf(
			`<w:p><w:pPr><w:keepNext/><w:spacing w:before="100" w:after="40"/><w:jc w:val="both"/><w:rPr>%s</w:rPr></w:pPr>`+
				`<w:r><w:rPr>%s</w:rPr><w:t>%s</w:t></w:r></w:p>`,
			rPr, rPr, escapeXML(heading),
		))
	}

	writeBullet := func(text string) {
		rPr := fmt.Sprintf(`<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:sz w:val="%d"/><w:szCs w:val="%d"/><w:lang w:val="en-IN"/>`,
			bodySz, bodySz)
		b.WriteString(fmt.Sprintf(
			`<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="9"/></w:numPr>`+
				`<w:spacing w:after="60"/><w:jc w:val="both"/><w:rPr>%s</w:rPr></w:pPr>`+
				`<w:r><w:rPr>%s</w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p>`,
			rPr, rPr, escapeXML(text),
		))
	}

	writeParagraph := func(text string) {
		rPr := fmt.Sprintf(`<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:sz w:val="%d"/><w:szCs w:val="%d"/><w:lang w:val="en-IN"/>`,
			bodySz, bodySz)
		b.WriteString(fmt.Sprintf(
			`<w:p><w:pPr><w:spacing w:after="80"/><w:jc w:val="both"/><w:rPr>%s</w:rPr></w:pPr>`+
				`<w:r><w:rPr>%s</w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p>`,
			rPr, rPr, escapeXML(text),
		))
	}

	for pi, proj := range projects {
		pName := strings.TrimSpace(proj.Project)
		if pName == "" {
			continue
		}
		writeProjectHeading(pName, fmt.Sprintf("proj_%d", pi+1))

		for ti, task := range proj.Tasks {
			tTitle := strings.TrimSpace(task.Title)
			if tTitle == "" {
				continue
			}
			writeTaskTitle(tTitle, fmt.Sprintf("task_%d_%d", pi+1, ti+1))

			sections := buildStructuredTaskSections(task)
			for _, sec := range sections {
				writeSubheading(sec.Heading)
				for _, item := range sec.Items {
					switch item.Kind {
					case blockBullet, blockNestedBullet:
						writeBullet(item.Text)
					case blockNumberedItem:
						if item.NumPrefix != "" {
							writeBullet(item.NumPrefix + " " + item.Text)
						} else {
							writeBullet(item.Text)
						}
					default:
						writeParagraph(item.Text)
					}
				}
			}
		}
	}

	if strings.TrimSpace(r.Conclusion) != "" {
		writeProjectHeading("Conclusion", "Conclusion")
		for _, para := range normalizeParagraphs(r.Conclusion) {
			writeParagraph(para)
		}
	}

	return b.String()
}

// generateDocxFromCustomDocxTemplate preserves an uploaded team .docx template's
// headers, footers, styles, numbering, media, margins, and page dimensions while
// injecting dynamic employee/month placeholders and structured project/task sections.
func generateDocxFromCustomDocxTemplate(r *models.Report, tmpl *models.Template, outputPath string) error {
	srcPath := filepath.Join(storage.TemplateDir(tmpl.ID), tmpl.SourceFile)
	masterBytes, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	zr, err := zip.NewReader(bytes.NewReader(masterBytes), int64(len(masterBytes)))
	if err != nil {
		return err
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	zw := zip.NewWriter(outFile)
	defer zw.Close()

	projects := make([]models.ReportSection, len(r.Sections))
	copy(projects, r.Sections)
	sort.Slice(projects, func(i, j int) bool { return projects[i].Order < projects[j].Order })

	projectNames := make([]string, 0, len(projects))
	for _, p := range projects {
		if strings.TrimSpace(p.Project) != "" {
			projectNames = append(projectNames, strings.TrimSpace(p.Project))
		}
	}
	projectsLine := formatCoverProjectsLine(projectNames)
	monthYear := strings.TrimSpace(r.CoverPage.MonthYear)
	monthLabel := r.CoverPage.Month
	yearLabel := r.CoverPage.Year
	if t, parseErr := time.Parse("2006-01", r.Month); parseErr == nil {
		if monthYear == "" {
			monthYear = t.Format("January 2006")
		}
		if monthLabel == "" {
			monthLabel = t.Format("January")
		}
		if yearLabel == "" {
			yearLabel = t.Format("2006")
		}
	}

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

		if f.Name == "word/document.xml" {
			s := string(raw)
			replacer := strings.NewReplacer(
				"{{EmployeeName}}", escapeXML(r.CoverPage.EmployeeName),
				"{{EmployeeCode}}", escapeXML(r.CoverPage.EmployeeCode),
				"{{AppraiserName}}", escapeXML(r.CoverPage.AppraiserName),
				"{{Month}}", escapeXML(monthLabel),
				"{{Year}}", escapeXML(yearLabel),
				"{{MonthYear}}", escapeXML(monthYear),
				"{{Projects}}", escapeXML(projectsLine),
			)
			s = replacer.Replace(s)

			bodyContent := buildERABodyContent(r, tmpl, projects)
			if strings.Contains(s, "{{REPORT_BODY}}") {
				s = strings.ReplaceAll(s, "{{REPORT_BODY}}", bodyContent)
			} else if sectIdx := strings.LastIndex(s, "<w:sectPr"); sectIdx != -1 {
				// Preserve cover/intro of the custom template and append structured report body before <w:sectPr>
				s = s[:sectIdx] + `<w:p><w:r><w:br w:type="page"/></w:r></w:p>` + bodyContent + s[sectIdx:]
			}
			raw = []byte(s)
		}

		w, err := zw.Create(f.Name)
		if err != nil {
			return err
		}
		if _, err := w.Write(raw); err != nil {
			return err
		}
	}
	return nil
}
