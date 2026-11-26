package services

import (
	"archive/zip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"performancereport/models"
	"performancereport/storage"
)

// resolveEffectiveTemplate loads the report's selected template (or default ERA template)
// and applies Custom Formatting overrides if r.FormattingMode == "custom".
func resolveEffectiveTemplate(r *models.Report) *models.Template {
	tmpl, err := GetTemplate(r.TemplateID)
	if err != nil || tmpl == nil {
		era := DefaultERATemplate()
		tmpl = &era
	}
	eff := *tmpl
	if strings.EqualFold(r.FormattingMode, "custom") {
		eff.Typography.ProjectName = models.TextStyle{FontFamily: "Century Gothic", FontSizePt: 23, Bold: true, Color: eff.PrimaryColor, Align: "left"}
		eff.Typography.TaskTitle = models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 16, Bold: true, Color: "#111111", Align: "left"}
		eff.Typography.Subheading = models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 14, Bold: true, Color: "#111111", Align: "left"}
		eff.Typography.TaskDescription = models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 14, Bold: false, Color: "#222222", Align: "left"}
		eff.Typography.ConclusionTitle = models.TextStyle{FontFamily: "Century Gothic", FontSizePt: 23, Bold: true, Color: eff.PrimaryColor, Align: "left"}
		eff.Typography.ConclusionBody = models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 14, Bold: false, Color: "#222222", Align: "justify"}
	}
	if eff.PageConfig.WidthMM <= 0 {
		eff.PageConfig.WidthMM = 215.9
	}
	if eff.PageConfig.HeightMM <= 0 {
		eff.PageConfig.HeightMM = 279.4
	}
	if eff.PageConfig.MarginTopMM <= 0 {
		eff.PageConfig.MarginTopMM = 20
	}
	if eff.PageConfig.MarginBottomMM <= 0 {
		eff.PageConfig.MarginBottomMM = 18
	}
	if eff.PageConfig.MarginLeftMM <= 0 {
		eff.PageConfig.MarginLeftMM = 20
	}
	if eff.PageConfig.MarginRightMM <= 0 {
		eff.PageConfig.MarginRightMM = 20
	}
	if eff.PageConfig.LineSpacing <= 0 {
		eff.PageConfig.LineSpacing = 1.45
	}
	return &eff
}

// resolveTemplateImages loads isolated images ONLY from the selected template's assets folder.
// For built-in ERA, falls back to embedded ERA assets if needed. Never leaks ERA assets to other templates.
func resolveTemplateImages(tmpl *models.Template) (coverBytes []byte, coverExt string, logoBytes []byte, logoExt string) {
	isERA := tmpl.ID == "era" || tmpl.SourceType == "era-builtin"

	if tmpl.Assets.CoverHeroImage != "" {
		p := filepath.Join(storage.TemplateAssetsDir(tmpl.ID), filepath.Base(tmpl.Assets.CoverHeroImage))
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			coverBytes = data
			coverExt = strings.TrimPrefix(strings.ToLower(filepath.Ext(tmpl.Assets.CoverHeroImage)), ".")
		}
	}
	if len(coverBytes) == 0 && isERA && CoverIllustrationBase64 != "" {
		coverBytes, _ = base64.StdEncoding.DecodeString(CoverIllustrationBase64)
		coverExt = "jpg"
	}

	if tmpl.Assets.LogoImage != "" {
		p := filepath.Join(storage.TemplateAssetsDir(tmpl.ID), filepath.Base(tmpl.Assets.LogoImage))
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			logoBytes = data
			logoExt = strings.TrimPrefix(strings.ToLower(filepath.Ext(tmpl.Assets.LogoImage)), ".")
		}
	}
	if len(logoBytes) == 0 && isERA && LogoBase64 != "" {
		logoBytes, _ = base64.StdEncoding.DecodeString(LogoBase64)
		logoExt = "png"
	}
	if coverExt == "jpeg" {
		coverExt = "jpg"
	}
	if logoExt == "jpeg" {
		logoExt = "jpg"
	}
	return
}

// -----------------------------------------------------------------------------
// Pagination & Automatic TOC Engine
// -----------------------------------------------------------------------------

type pageBlockKind string

const (
	blockProjectHeader  pageBlockKind = "project_header"
	blockTaskTitle      pageBlockKind = "task_title"
	blockSubheading     pageBlockKind = "subheading"
	blockBullet         pageBlockKind = "bullet"
	blockNumberedItem   pageBlockKind = "numbered"
	blockNestedBullet   pageBlockKind = "nested_bullet"
	blockParagraph      pageBlockKind = "paragraph"
	blockConclusionHead pageBlockKind = "conclusion_head"
	blockConclusionPara pageBlockKind = "conclusion_para"
)

type pageBlock struct {
	Kind       pageBlockKind
	Text       string
	ProjectIdx int
	TaskIdx    int
	NumPrefix  string
	HeightMM   float64
}

type paginatedPage struct {
	PageNumber int
	Blocks     []pageBlock
}

type tocTaskEntry struct {
	Title      string
	PageNumber int
}

type tocProjectEntry struct {
	Project    string
	PageNumber int
	Tasks      []tocTaskEntry
}

type paginatedDocument struct {
	Pages          []paginatedPage
	TOCProjects    []tocProjectEntry
	ConclusionPage int
	TotalPages     int
}

func estimateLines(text string, fontSizePt float64, contentWidthMM float64) int {
	if strings.TrimSpace(text) == "" {
		return 1
	}
	charWidthMM := fontSizePt * 0.41 * 0.352778
	if charWidthMM <= 0 {
		charWidthMM = 2.0
	}
	charsPerLine := int(math.Floor(contentWidthMM / charWidthMM))
	if charsPerLine < 30 {
		charsPerLine = 30
	}
	lines := (len([]rune(text)) + charsPerLine - 1) / charsPerLine
	if lines < 1 {
		lines = 1
	}
	return lines
}

func paginateReport(r *models.Report, tmpl *models.Template) *paginatedDocument {
	contentWidthMM := tmpl.PageConfig.WidthMM - tmpl.PageConfig.MarginLeftMM - tmpl.PageConfig.MarginRightMM
	if contentWidthMM < 80 {
		contentWidthMM = 175
	}
	usableHeightMM := tmpl.PageConfig.HeightMM - tmpl.PageConfig.MarginTopMM - tmpl.PageConfig.MarginBottomMM - 8.0
	if usableHeightMM < 120 {
		usableHeightMM = 233
	}

	lineHeightFactor := tmpl.PageConfig.LineSpacing
	if lineHeightFactor <= 0 {
		lineHeightFactor = 1.45
	}

	projLineH := tmpl.Typography.ProjectName.FontSizePt * 0.352778 * 1.25
	taskLineH := tmpl.Typography.TaskTitle.FontSizePt * 0.352778 * 1.28
	subLineH := tmpl.Typography.Subheading.FontSizePt * 0.352778 * 1.28
	bodyLineH := tmpl.Typography.TaskDescription.FontSizePt * 0.352778 * lineHeightFactor

	doc := &paginatedDocument{}
	currentPageNum := 3
	var currentBlocks []pageBlock
	usedHeight := 0.0

	flushPage := func() {
		if len(currentBlocks) > 0 {
			doc.Pages = append(doc.Pages, paginatedPage{
				PageNumber: currentPageNum,
				Blocks:     currentBlocks,
			})
			currentPageNum++
			currentBlocks = nil
			usedHeight = 0.0
		}
	}

	addBlock := func(b pageBlock) int {
		if usedHeight+b.HeightMM > usableHeightMM && len(currentBlocks) > 0 {
			flushPage()
		}
		assignedPage := currentPageNum
		currentBlocks = append(currentBlocks, b)
		usedHeight += b.HeightMM
		return assignedPage
	}

	for pIdx, sec := range r.Sections {
		if !IsValidProjectName(sec.Project) {
			continue
		}
		if len(currentBlocks) > 0 {
			flushPage()
		}

		projTitle := cleanProjectTitle(sec.Project)
		pLines := estimateLines(projTitle, tmpl.Typography.ProjectName.FontSizePt, contentWidthMM)
		projPage := addBlock(pageBlock{
			Kind:       blockProjectHeader,
			Text:       projTitle,
			ProjectIdx: pIdx,
			HeightMM:   float64(pLines)*projLineH + 7.0,
		})

		tocProj := tocProjectEntry{
			Project:    projTitle,
			PageNumber: projPage,
		}

		for tIdx, task := range sec.Tasks {
			tTitle := cleanDisplayTitle(task.Title)
			tLines := estimateLines(tTitle, tmpl.Typography.TaskTitle.FontSizePt, contentWidthMM-6)
			tHeight := float64(tLines)*taskLineH + 4.5

			if usedHeight+tHeight+14.0 > usableHeightMM && len(currentBlocks) > 0 {
				flushPage()
			}

			taskPage := addBlock(pageBlock{
				Kind:       blockTaskTitle,
				Text:       tTitle,
				ProjectIdx: pIdx,
				TaskIdx:    tIdx,
				HeightMM:   tHeight,
			})
			tocProj.Tasks = append(tocProj.Tasks, tocTaskEntry{
				Title:      tTitle,
				PageNumber: taskPage,
			})

			subSections := buildStructuredTaskSections(task)
			for _, subSec := range subSections {
				if len(subSec.Items) == 0 {
					continue
				}
				subH := subLineH + 3.5
				if usedHeight+subH+bodyLineH+2.0 > usableHeightMM && len(currentBlocks) > 0 {
					flushPage()
				}
				addBlock(pageBlock{
					Kind:     blockSubheading,
					Text:     subSec.Heading,
					HeightMM: subH,
				})
				for _, item := range subSec.Items {
					if item.Kind == blockParagraph {
						pLines := estimateLines(item.Text, tmpl.Typography.TaskDescription.FontSizePt, contentWidthMM)
						addBlock(pageBlock{
							Kind:     blockParagraph,
							Text:     item.Text,
							HeightMM: float64(pLines)*bodyLineH + 2.2,
						})
					} else {
						indentW := contentWidthMM - 8.0
						if item.Kind == blockNestedBullet {
							indentW -= 6.0
						}
						bLines := estimateLines(item.Text, tmpl.Typography.TaskDescription.FontSizePt, indentW)
						addBlock(pageBlock{
							Kind:      item.Kind,
							Text:      item.Text,
							NumPrefix: item.NumPrefix,
							HeightMM:  float64(bLines)*bodyLineH + 1.5,
						})
					}
				}
			}

			usedHeight += 3.5
		}

		doc.TOCProjects = append(doc.TOCProjects, tocProj)
	}
	flushPage()

	if strings.TrimSpace(r.Conclusion) != "" {
		cHeadH := projLineH + 7.0
		doc.ConclusionPage = addBlock(pageBlock{
			Kind:     blockConclusionHead,
			Text:     "Conclusion",
			HeightMM: cHeadH,
		})
		for _, p := range normalizeParagraphs(r.Conclusion) {
			pLines := estimateLines(p, tmpl.Typography.ConclusionBody.FontSizePt, contentWidthMM)
			addBlock(pageBlock{
				Kind:     blockConclusionPara,
				Text:     p,
				HeightMM: float64(pLines)*bodyLineH + 3.0,
			})
		}
		flushPage()
	}

	doc.TotalPages = 2 + len(doc.Pages)
	return doc
}

type parsedLineItem struct {
	Kind      pageBlockKind
	Text      string
	NumPrefix string
}

type structuredSubSection struct {
	Heading string
	Items   []parsedLineItem
}

var numberedLineRe = regexp.MustCompile(`^(\d+[\.\)])\s+(.*)$`)

func detectEmbeddedSectionHeading(line string) (string, bool) {
	cleaned := strings.TrimSpace(stripMarkdownFormatting(line))
	cleaned = strings.TrimPrefix(cleaned, "•")
	cleaned = strings.TrimPrefix(cleaned, "-")
	cleaned = strings.TrimPrefix(cleaned, "*")
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.TrimRight(cleaned, ":-–— ")
	low := strings.ToLower(cleaned)
	switch low {
	case "activities performed", "activity performed":
		return "Activities Performed", true
	case "issue identified", "issues identified":
		return "Issue Identified", true
	case "solution implemented", "solutions implemented":
		return "Solution Implemented", true
	case "outcome", "outcomes":
		return "Outcome", true
	case "challenges faced", "challenge faced", "challenges":
		return "Challenges Faced", true
	default:
		return "", false
	}
}

func buildStructuredTaskSections(task models.ReportTask) []structuredSubSection {
	var result []structuredSubSection
	appendToSection := func(heading string, items []parsedLineItem) {
		if len(items) == 0 {
			return
		}
		for i := range result {
			if strings.EqualFold(result[i].Heading, heading) {
				result[i].Items = append(result[i].Items, items...)
				return
			}
		}
		result = append(result, structuredSubSection{Heading: heading, Items: items})
	}

	parseIntoSections := func(defaultHeading, raw string, paragraphMode bool) {
		if strings.TrimSpace(raw) == "" {
			return
		}
		lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
		curHeading := defaultHeading
		var curItems []parsedLineItem
		var curText strings.Builder
		var curKind pageBlockKind
		var curNum string

		flushItem := func() {
			txt := strings.TrimSpace(stripMarkdownFormatting(curText.String()))
			if txt != "" {
				k := curKind
				if k == "" {
					if paragraphMode || strings.EqualFold(curHeading, "Outcome") {
						k = blockParagraph
					} else {
						k = blockBullet
					}
				}
				curItems = append(curItems, parsedLineItem{
					Kind:      k,
					Text:      txt,
					NumPrefix: curNum,
				})
			}
			curText.Reset()
			curKind = ""
			curNum = ""
		}

		flushSection := func() {
			flushItem()
			if len(curItems) > 0 {
				appendToSection(curHeading, curItems)
				curItems = nil
			}
		}

		for _, rawLine := range lines {
			trimmedRight := strings.TrimRight(rawLine, " \t\r")
			trimmed := strings.TrimSpace(trimmedRight)
			if trimmed == "" {
				flushItem()
				continue
			}

			// 1. Check if line is an embedded section heading (e.g. "Activities Performed", "• Outcome")
			if heading, ok := detectEmbeddedSectionHeading(trimmed); ok {
				flushSection()
				curHeading = heading
				paragraphMode = strings.EqualFold(heading, "Outcome")
				continue
			}

			leadingSpaces := len(trimmedRight) - len(strings.TrimLeft(trimmedRight, " \t"))

			// 2. Nested bullet (indented or 'o ')
			if strings.HasPrefix(trimmed, "o ") || strings.HasPrefix(trimmed, "◦ ") ||
				(leadingSpaces >= 2 && (strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "• "))) {
				flushItem()
				clean := strings.TrimPrefix(trimmed, "o ")
				clean = strings.TrimPrefix(clean, "◦ ")
				clean = strings.TrimPrefix(clean, "- ")
				clean = strings.TrimPrefix(clean, "* ")
				clean = strings.TrimPrefix(clean, "• ")
				curKind = blockNestedBullet
				curText.WriteString(strings.TrimSpace(clean))
				continue
			}

			// 3. Numbered list item
			if m := numberedLineRe.FindStringSubmatch(trimmed); len(m) == 3 {
				flushItem()
				curKind = blockNumberedItem
				curNum = m[1]
				curText.WriteString(strings.TrimSpace(m[2]))
				continue
			}

			// 4. Explicit new bullet
			if strings.HasPrefix(trimmed, "•") || strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || trimmed == "-" || trimmed == "*" {
				flushItem()
				clean := strings.TrimPrefix(trimmed, "•")
				clean = strings.TrimPrefix(clean, "-")
				clean = strings.TrimPrefix(clean, "*")
				curKind = blockBullet
				curText.WriteString(strings.TrimSpace(clean))
				continue
			}

			// 5. Continuation line (no bullet prefix): merge onto previous bullet/paragraph!
			if curText.Len() > 0 {
				curText.WriteByte(' ')
				curText.WriteString(trimmed)
			} else {
				if paragraphMode || strings.EqualFold(curHeading, "Outcome") {
					curKind = blockParagraph
				} else {
					curKind = blockBullet
				}
				curText.WriteString(trimmed)
			}
		}
		flushSection()
	}

	parseIntoSections("Issue Identified", task.IssueIdentified, false)
	parseIntoSections("Activities Performed", task.ActivitiesPerformed, false)
	parseIntoSections("Solution Implemented", task.SolutionImplemented, false)
	parseIntoSections("Challenges Faced", task.ChallengesFaced, false)
	for _, cs := range task.CustomSections {
		if strings.TrimSpace(cs.Heading) != "" && strings.TrimSpace(cs.Content) != "" {
			parseIntoSections(cs.Heading, cs.Content, false)
		}
	}
	parseIntoSections("Outcome", task.Outcome, true)
	return result
}

func normalizeParagraphs(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	chunks := regexp.MustCompile(`\n\s*\n`).Split(raw, -1)
	var out []string
	for _, chunk := range chunks {
		lines := strings.Split(chunk, "\n")
		var buf strings.Builder
		for _, l := range lines {
			t := strings.TrimSpace(stripMarkdownFormatting(l))
			if t == "" {
				continue
			}
			if buf.Len() > 0 {
				prev := buf.String()
				lastChar := prev[len(prev)-1]
				firstRune := rune(t[0])
				// Only split into a new paragraph if previous line ended a sentence AND next line looks like a fresh paragraph start after a long line
				if (lastChar == '.' || lastChar == '!' || lastChar == '?') && len(prev) > 180 && firstRune >= 'A' && firstRune <= 'Z' {
					out = append(out, prev)
					buf.Reset()
				} else {
					buf.WriteByte(' ')
				}
			}
			buf.WriteString(t)
		}
		if buf.Len() > 0 {
			out = append(out, buf.String())
		}
	}
	return out
}

func parseFormattedLines(text string) []parsedLineItem {
	secs := buildStructuredTaskSections(models.ReportTask{ActivitiesPerformed: text})
	var items []parsedLineItem
	for _, s := range secs {
		items = append(items, s.Items...)
	}
	return items
}

// -----------------------------------------------------------------------------
// DOCX Generation (with exact pt half-points & template assets)
// -----------------------------------------------------------------------------

// GenerateDocx creates a Word document (.docx) using the selected template's exact typography & assets.
func GenerateDocx(r *models.Report) (string, error) {
	exportsDir := storage.ExportsDir()
	if err := storage.EnsureDir(exportsDir); err != nil {
		return "", err
	}

	t, _ := time.Parse("2006-01", r.Month)
	filename := fmt.Sprintf("PerformanceReport%s.docx", t.Format("January2006"))
	outputPath := filepath.Join(exportsDir, filename)

	if err := buildDocx(r, outputPath); err != nil {
		return "", err
	}
	return outputPath, nil
}

func buildDocx(r *models.Report, outputPath string) error {
	tmpl := resolveEffectiveTemplate(r)
	if tmpl == nil || tmpl.ID == "era" {
		if err := generateDocxFromMasterTemplate(r, tmpl, outputPath); err == nil {
			return nil
		}
	} else if tmpl.SourceType == "docx" && tmpl.SourceFile != "" {
		if err := generateDocxFromCustomDocxTemplate(r, tmpl, outputPath); err == nil {
			return nil
		}
	}
	coverBytes, coverExt, logoBytes, logoExt := resolveTemplateImages(tmpl)
	paginated := paginateReport(r, tmpl)

	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	parts := docxParts(r, tmpl, paginated, len(coverBytes) > 0, coverExt, len(logoBytes) > 0, logoExt)
	for name, content := range parts {
		fw, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(fw, content); err != nil {
			return err
		}
	}

	if len(coverBytes) > 0 {
		fw, err := zw.Create("word/media/cover_hero." + coverExt)
		if err == nil {
			_, _ = fw.Write(coverBytes)
		}
	}
	if len(logoBytes) > 0 {
		fw, err := zw.Create("word/media/team_logo." + logoExt)
		if err == nil {
			_, _ = fw.Write(logoBytes)
		}
	}

	return nil
}

func ptToHalfPt(pt float64) int {
	if pt <= 0 {
		return 28
	}
	return int(math.Round(pt * 2.0))
}

func hexColor(c, fallback string) string {
	c = strings.TrimPrefix(strings.TrimSpace(c), "#")
	if len(c) == 6 {
		return strings.ToUpper(c)
	}
	return fallback
}

func docxParts(r *models.Report, tmpl *models.Template, paginated *paginatedDocument, hasCover bool, coverExt string, hasLogo bool, logoExt string) map[string]string {
	body := buildDocBody(r, tmpl, paginated, hasCover, hasLogo)
	return map[string]string{
		"[Content_Types].xml":          contentTypesXML(),
		"_rels/.rels":                  relsXML(),
		"word/document.xml":            documentXML(body, tmpl),
		"word/_rels/document.xml.rels": documentRels(hasCover, coverExt, hasLogo, logoExt),
		"word/header1.xml":             headerXML(tmpl),
		"word/footer1.xml":             footerXML(tmpl),
		"word/styles.xml":              stylesXML(tmpl),
		"word/settings.xml":            settingsXML(),
		"word/theme/theme1.xml":        themeXML(tmpl),
		"docProps/core.xml":            coreXML(r),
		"docProps/app.xml":             appXML(),
	}
}

func buildDocBody(r *models.Report, tmpl *models.Template, paginated *paginatedDocument, hasCover bool, hasLogo bool) string {
	var sb strings.Builder
	cp := r.CoverPage
	primaryHex := hexColor(tmpl.PrimaryColor, "262262")

	// ----------------------------------------------------
	// PAGE 1: COVER PAGE
	// ----------------------------------------------------
	if hasCover {
		sb.WriteString(`<w:p>
  <w:pPr><w:jc w:val="center"/><w:spacing w:before="0" w:after="200"/></w:pPr>
  <w:r>
    <w:drawing>
      <wp:inline distT="0" distB="0" distL="0" distR="0">
        <wp:extent cx="5400000" cy="2700000"/>
        <wp:docPr id="1" name="Cover Illustration"/>
        <a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
          <a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">
            <pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">
              <pic:nvPicPr>
                <pic:cNvPr id="0" name="cover_hero"/>
                <pic:cNvPicPr/>
              </pic:nvPicPr>
              <pic:blipFill>
                <a:blip r:embed="rId4" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/>
                <a:stretch><a:fillRect/></a:stretch>
              </pic:blipFill>
              <pic:spPr>
                <a:xfrm><a:off x="0" y="0"/><a:ext cx="5400000" cy="2700000"/></a:xfrm>
                <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
              </pic:spPr>
            </pic:pic>
          </a:graphicData>
        </a:graphic>
      </wp:inline>
    </w:drawing>
  </w:r>
</w:p>`)
	}

	bannerFont := tmpl.Typography.BannerTitle.FontFamily
	if bannerFont == "" {
		bannerFont = "Century Gothic"
	}
	bannerSz := ptToHalfPt(tmpl.Typography.BannerTitle.FontSizePt)

	bannerLines := []string{"Monthly", "Performance Report"}
	monthYearText := cp.MonthYear
	if monthYearText == "" {
		monthYearText = strings.TrimSpace(cp.Month + " " + cp.Year)
	}
	bannerLines = append(bannerLines, monthYearText)

	for idx, line := range bannerLines {
		before := 0
		after := 60
		if idx == 0 {
			before = 240
		}
		if idx == len(bannerLines)-1 {
			after = 240
		}
		sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr>
    <w:shd w:val="clear" w:color="auto" w:fill="%s"/>
    <w:spacing w:before="%d" w:after="%d"/>
    <w:ind w:left="360" w:right="360"/>
  </w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:b/>
      <w:color w:val="FFFFFF"/>
      <w:sz w:val="%d"/>
    </w:rPr>
    <w:t>%s</w:t>
  </w:r>
</w:p>`, primaryHex, before, after, xmlEscape(bannerFont), xmlEscape(bannerFont), xmlEscape(bannerFont), bannerSz, xmlEscape(line)))
	}

	sb.WriteString(spacer())

	validProjects := extractValidCoverProjects(cp.Projects)
	projectLine := formatCoverProjectsLine(validProjects)
	projCoverFont := tmpl.Typography.CoverProjects.FontFamily
	if projCoverFont == "" {
		projCoverFont = "Century Gothic"
	}
	projCoverSz := ptToHalfPt(tmpl.Typography.CoverProjects.FontSizePt)
	projCoverHex := hexColor(tmpl.Typography.CoverProjects.Color, primaryHex)

	if projectLine != "" {
		sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr><w:spacing w:before="240" w:after="240"/><w:ind w:left="360"/></w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:b/>
      <w:color w:val="%s"/>
      <w:sz w:val="%d"/>
    </w:rPr>
    <w:t>Projects : %s</w:t>
  </w:r>
</w:p>`, xmlEscape(projCoverFont), xmlEscape(projCoverFont), xmlEscape(projCoverFont), projCoverHex, projCoverSz, xmlEscape(projectLine)))
	}

	sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr>
    <w:pBdr><w:bottom w:val="single" w:sz="12" w:space="1" w:color="%s"/></w:pBdr>
    <w:spacing w:before="120" w:after="240"/>
    <w:ind w:left="360" w:right="360"/>
  </w:pPr>
</w:p>`, primaryHex))

	empFont := tmpl.Typography.CoverEmployee.FontFamily
	if empFont == "" {
		empFont = "Avenir Next LT Pro Light"
	}
	empSz := ptToHalfPt(tmpl.Typography.CoverEmployee.FontSizePt)
	empHex := hexColor(tmpl.Typography.CoverEmployee.Color, "111111")

	logoCell := `<w:p/>`
	if hasLogo {
		logoEmbed := "rId5"
		if !hasCover {
			logoEmbed = "rId4"
		}
		logoCell = fmt.Sprintf(`<w:p>
        <w:pPr><w:jc w:val="right"/></w:pPr>
        <w:r>
          <w:drawing>
            <wp:inline distT="0" distB="0" distL="0" distR="0">
              <wp:extent cx="1000000" cy="612500"/>
              <wp:docPr id="2" name="Team Logo"/>
              <a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
                <a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">
                  <pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">
                    <pic:nvPicPr>
                      <pic:cNvPr id="1" name="team_logo"/>
                      <pic:cNvPicPr/>
                    </pic:nvPicPr>
                    <pic:blipFill>
                      <a:blip r:embed="%s" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/>
                      <a:stretch><a:fillRect/></a:stretch>
                    </pic:blipFill>
                    <pic:spPr>
                      <a:xfrm><a:off x="0" y="0"/><a:ext cx="1000000" cy="612500"/></a:xfrm>
                      <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                    </pic:spPr>
                  </pic:pic>
                </a:graphicData>
              </a:graphic>
            </wp:inline>
          </w:drawing>
        </w:r>
      </w:p>`, logoEmbed)
	}

	sb.WriteString(fmt.Sprintf(`<w:tbl>
  <w:tblPr>
    <w:tblW w:w="9200" w:type="dxa"/>
    <w:tblBorders>
      <w:top w:val="none"/><w:left w:val="none"/><w:bottom w:val="none"/>
      <w:right w:val="none"/><w:insideH w:val="none"/><w:insideV w:val="none"/>
    </w:tblBorders>
  </w:tblPr>
  <w:tblGrid><w:gridCol w:w="6200"/><w:gridCol w:w="3000"/></w:tblGrid>
  <w:tr>
    <w:tc>
      <w:tcPr><w:tcW w:w="6200" w:type="dxa"/></w:tcPr>
      <w:p>
        <w:pPr><w:spacing w:before="60" w:after="60"/><w:ind w:left="360"/></w:pPr>
        <w:r><w:rPr><w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/><w:color w:val="%s"/><w:sz w:val="%d"/></w:rPr><w:t>Employee Name: </w:t></w:r>
        <w:r><w:rPr><w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/><w:b/><w:color w:val="%s"/><w:sz w:val="%d"/></w:rPr><w:t>%s</w:t></w:r>
      </w:p>
      <w:p>
        <w:pPr><w:spacing w:before="60" w:after="60"/><w:ind w:left="360"/></w:pPr>
        <w:r><w:rPr><w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/><w:color w:val="%s"/><w:sz w:val="%d"/></w:rPr><w:t>Employee Code: </w:t></w:r>
        <w:r><w:rPr><w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/><w:b/><w:color w:val="%s"/><w:sz w:val="%d"/></w:rPr><w:t>%s</w:t></w:r>
      </w:p>
      <w:p>
        <w:pPr><w:spacing w:before="60" w:after="60"/><w:ind w:left="360"/></w:pPr>
        <w:r><w:rPr><w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/><w:color w:val="%s"/><w:sz w:val="%d"/></w:rPr><w:t>Appraiser Name: </w:t></w:r>
        <w:r><w:rPr><w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/><w:b/><w:color w:val="%s"/><w:sz w:val="%d"/></w:rPr><w:t>%s</w:t></w:r>
      </w:p>
    </w:tc>
    <w:tc>
      <w:tcPr><w:tcW w:w="3000" w:type="dxa"/><w:vAlign w:val="center"/></w:tcPr>
      %s
    </w:tc>
  </w:tr>
</w:tbl>`,
		empFont, empFont, empFont, empHex, empSz, empFont, empFont, empFont, empHex, empSz, xmlEscape(cp.EmployeeName),
		empFont, empFont, empFont, empHex, empSz, empFont, empFont, empFont, empHex, empSz, xmlEscape(cp.EmployeeCode),
		empFont, empFont, empFont, empHex, empSz, empFont, empFont, empFont, empHex, empSz, xmlEscape(cp.AppraiserName),
		logoCell,
	))

	sb.WriteString(pageBreak())

	// ----------------------------------------------------
	// PAGE 2: AUTOMATIC TABLE OF CONTENTS WITH PAGE NUMBERS
	// ----------------------------------------------------
	tocTitleFont := tmpl.Typography.TOCTitle.FontFamily
	tocTitleSz := ptToHalfPt(tmpl.Typography.TOCTitle.FontSizePt)
	tocTitleHex := hexColor(tmpl.Typography.TOCTitle.Color, primaryHex)

	tocProjFont := tmpl.Typography.TOCProject.FontFamily
	tocProjSz := ptToHalfPt(tmpl.Typography.TOCProject.FontSizePt)
	tocProjHex := hexColor(tmpl.Typography.TOCProject.Color, "333333")

	tocTaskFont := tmpl.Typography.TOCTask.FontFamily
	tocTaskSz := ptToHalfPt(tmpl.Typography.TOCTask.FontSizePt)
	tocTaskHex := hexColor(tmpl.Typography.TOCTask.Color, "333333")

	sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr><w:spacing w:before="240" w:after="240"/><w:ind w:left="360"/></w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:b/>
      <w:color w:val="%s"/>
      <w:sz w:val="%d"/>
    </w:rPr>
    <w:t>TABLE OF CONTENTS</w:t>
  </w:r>
</w:p>`, xmlEscape(tocTitleFont), xmlEscape(tocTitleFont), xmlEscape(tocTitleFont), tocTitleHex, tocTitleSz))

	for _, tocProj := range paginated.TOCProjects {
		sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr>
    <w:spacing w:before="180" w:after="80"/><w:ind w:left="400"/>
  </w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:b/>
      <w:color w:val="%s"/>
      <w:sz w:val="%d"/>
    </w:rPr>
    <w:t>▪  %s</w:t>
  </w:r>
</w:p>`, xmlEscape(tocProjFont), xmlEscape(tocProjFont), xmlEscape(tocProjFont), tocProjHex, tocProjSz, xmlEscape(tocProj.Project)))

		for _, tk := range tocProj.Tasks {
			sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr>
    <w:spacing w:before="40" w:after="40"/><w:ind w:left="800"/>
  </w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:color w:val="%s"/>
      <w:sz w:val="%d"/>
    </w:rPr>
    <w:t>•  %s</w:t>
  </w:r>
</w:p>`, xmlEscape(tocTaskFont), xmlEscape(tocTaskFont), xmlEscape(tocTaskFont), tocTaskHex, tocTaskSz, xmlEscape(tk.Title)))
		}
	}

	if paginated.ConclusionPage > 0 {
		sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr>
    <w:spacing w:before="180" w:after="80"/><w:ind w:left="400"/>
  </w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:b/>
      <w:color w:val="%s"/>
      <w:sz w:val="%d"/>
    </w:rPr>
    <w:t>▪  Conclusion</w:t>
  </w:r>
</w:p>`, xmlEscape(tocProjFont), xmlEscape(tocProjFont), xmlEscape(tocProjFont), tocProjHex, tocProjSz))
	}

	sb.WriteString(pageBreak())

	// ----------------------------------------------------
	// PAGES 3+: DETAILED PROJECT & TASK SECTIONS
	// Exact font sizes: Project=23pt (sz=46), TaskTitle=16pt (sz=32), Subheading/Desc=14pt (sz=28)
	// ----------------------------------------------------
	projFont := tmpl.Typography.ProjectName.FontFamily
	projSz := ptToHalfPt(tmpl.Typography.ProjectName.FontSizePt)
	projHex := hexColor(tmpl.Typography.ProjectName.Color, primaryHex)

	taskFont := tmpl.Typography.TaskTitle.FontFamily
	taskSz := ptToHalfPt(tmpl.Typography.TaskTitle.FontSizePt)
	taskHex := hexColor(tmpl.Typography.TaskTitle.Color, "111111")

	subFont := tmpl.Typography.Subheading.FontFamily
	subSz := ptToHalfPt(tmpl.Typography.Subheading.FontSizePt)
	subHex := hexColor(tmpl.Typography.Subheading.Color, "111111")

	descFont := tmpl.Typography.TaskDescription.FontFamily
	descSz := ptToHalfPt(tmpl.Typography.TaskDescription.FontSizePt)
	descHex := hexColor(tmpl.Typography.TaskDescription.Color, "222222")

	sectionIdx := 0
	for _, section := range r.Sections {
		if !IsValidProjectName(section.Project) {
			continue
		}
		if sectionIdx > 0 {
			sb.WriteString(pageBreak())
		}
		sectionIdx++

		sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr><w:spacing w:before="200" w:after="200"/><w:ind w:left="360"/></w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:b/>
      <w:color w:val="%s"/>
      <w:sz w:val="%d"/>
    </w:rPr>
    <w:t>%s</w:t>
  </w:r>
</w:p>`, xmlEscape(projFont), xmlEscape(projFont), xmlEscape(projFont), projHex, projSz, xmlEscape(cleanProjectTitle(section.Project))))

		for _, task := range section.Tasks {
			sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr><w:spacing w:before="200" w:after="100"/><w:ind w:left="360"/></w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:b/>
      <w:color w:val="%s"/>
      <w:sz w:val="%d"/>
    </w:rPr>
    <w:t>•  %s</w:t>
  </w:r>
</w:p>`, xmlEscape(taskFont), xmlEscape(taskFont), xmlEscape(taskFont), taskHex, taskSz, xmlEscape(cleanDisplayTitle(task.Title))))

			for _, subSec := range buildStructuredTaskSections(task) {
				if len(subSec.Items) == 0 {
					continue
				}
				writeTaskStructuredItems(&sb, subSec.Heading, subSec.Items, subFont, subSz, subHex, descFont, descSz, descHex)
			}

			sb.WriteString(spacer())
		}
	}

	if strings.TrimSpace(r.Conclusion) != "" {
		sb.WriteString(pageBreak())
		cTitleFont := tmpl.Typography.ConclusionTitle.FontFamily
		cTitleSz := ptToHalfPt(tmpl.Typography.ConclusionTitle.FontSizePt)
		cTitleHex := hexColor(tmpl.Typography.ConclusionTitle.Color, primaryHex)
		cBodyFont := tmpl.Typography.ConclusionBody.FontFamily
		cBodySz := ptToHalfPt(tmpl.Typography.ConclusionBody.FontSizePt)
		cBodyHex := hexColor(tmpl.Typography.ConclusionBody.Color, "222222")

		sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr><w:spacing w:before="200" w:after="200"/><w:ind w:left="360"/></w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:b/>
      <w:color w:val="%s"/>
      <w:sz w:val="%d"/>
    </w:rPr>
    <w:t>Conclusion</w:t>
  </w:r>
</w:p>`, xmlEscape(cTitleFont), xmlEscape(cTitleFont), xmlEscape(cTitleFont), cTitleHex, cTitleSz))

		for _, p := range normalizeParagraphs(r.Conclusion) {
			sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr><w:spacing w:before="60" w:after="120"/><w:ind w:left="360" w:right="360"/><w:jc w:val="both"/></w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:color w:val="%s"/>
      <w:sz w:val="%d"/>
    </w:rPr>
    <w:t>%s</w:t>
  </w:r>
</w:p>`, xmlEscape(cBodyFont), xmlEscape(cBodyFont), xmlEscape(cBodyFont), cBodyHex, cBodySz, xmlEscape(p)))
		}
	}

	return sb.String()
}

func writeTaskStructuredItems(sb *strings.Builder, label string, items []parsedLineItem, subFont string, subSz int, subHex, descFont string, descSz int, descHex string) {
	sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr><w:spacing w:before="140" w:after="60"/><w:ind w:left="360"/></w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:b/>
      <w:color w:val="%s"/>
      <w:sz w:val="%d"/>
    </w:rPr>
    <w:t>%s</w:t>
  </w:r>
</w:p>`, xmlEscape(subFont), xmlEscape(subFont), xmlEscape(subFont), subHex, subSz, xmlEscape(label)))

	for _, item := range items {
		if item.Kind == blockParagraph {
			sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr><w:spacing w:before="40" w:after="100"/><w:ind w:left="360" w:right="360"/></w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:sz w:val="%d"/>
      <w:color w:val="%s"/>
    </w:rPr>
    <w:t>%s</w:t>
  </w:r>
</w:p>`, xmlEscape(descFont), xmlEscape(descFont), xmlEscape(descFont), descSz, descHex, xmlEscape(item.Text)))
			continue
		}
		indent := 720
		bullet := "• "
		switch item.Kind {
		case blockNestedBullet:
			indent = 1080
			bullet = "o "
		case blockNumberedItem:
			bullet = item.NumPrefix + " "
		}
		sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr>
    <w:spacing w:before="40" w:after="40"/>
    <w:ind w:left="%d" w:hanging="240"/>
  </w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:sz w:val="%d"/>
      <w:color w:val="%s"/>
    </w:rPr>
    <w:t>%s</w:t>
  </w:r>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:sz w:val="%d"/>
      <w:color w:val="%s"/>
    </w:rPr>
    <w:t>%s</w:t>
  </w:r>
</w:p>`, indent,
			xmlEscape(descFont), xmlEscape(descFont), xmlEscape(descFont), descSz, descHex, xmlEscape(bullet),
			xmlEscape(descFont), xmlEscape(descFont), xmlEscape(descFont), descSz, descHex, xmlEscape(item.Text)))
	}
}

func writeTaskSubSectionStyled(sb *strings.Builder, label, text, subFont string, subSz int, subHex, descFont string, descSz int, descHex string) {
	sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr><w:spacing w:before="140" w:after="60"/><w:ind w:left="360"/></w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:b/>
      <w:color w:val="%s"/>
      <w:sz w:val="%d"/>
    </w:rPr>
    <w:t>%s</w:t>
  </w:r>
</w:p>`, xmlEscape(subFont), xmlEscape(subFont), xmlEscape(subFont), subHex, subSz, xmlEscape(label)))

	for _, item := range parseFormattedLines(text) {
		indent := 720
		bullet := "• "
		switch item.Kind {
		case blockNestedBullet:
			indent = 1080
			bullet = "o "
		case blockNumberedItem:
			bullet = item.NumPrefix + " "
		}
		sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr>
    <w:spacing w:before="40" w:after="40"/>
    <w:ind w:left="%d" w:hanging="240"/>
  </w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:sz w:val="%d"/>
      <w:color w:val="%s"/>
    </w:rPr>
    <w:t>%s</w:t>
  </w:r>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:sz w:val="%d"/>
      <w:color w:val="%s"/>
    </w:rPr>
    <w:t>%s</w:t>
  </w:r>
</w:p>`, indent,
			xmlEscape(descFont), xmlEscape(descFont), xmlEscape(descFont), descSz, descHex, xmlEscape(bullet),
			xmlEscape(descFont), xmlEscape(descFont), xmlEscape(descFont), descSz, descHex, xmlEscape(item.Text)))
	}
}

func writeTaskOutcomeStyled(sb *strings.Builder, outcome, subFont string, subSz int, subHex, descFont string, descSz int, descHex string) {
	sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr><w:spacing w:before="140" w:after="60"/><w:ind w:left="360"/></w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:b/>
      <w:color w:val="%s"/>
      <w:sz w:val="%d"/>
    </w:rPr>
    <w:t>Outcome</w:t>
  </w:r>
</w:p>`, xmlEscape(subFont), xmlEscape(subFont), xmlEscape(subFont), subHex, subSz))

	for _, para := range strings.Split(outcome, "\n") {
		p := strings.TrimSpace(stripMarkdownFormatting(para))
		if p == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf(`<w:p>
  <w:pPr><w:spacing w:before="40" w:after="100"/><w:ind w:left="360" w:right="360"/></w:pPr>
  <w:r>
    <w:rPr>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:sz w:val="%d"/>
      <w:color w:val="%s"/>
    </w:rPr>
    <w:t>%s</w:t>
  </w:r>
</w:p>`, xmlEscape(descFont), xmlEscape(descFont), xmlEscape(descFont), descSz, descHex, xmlEscape(p)))
	}
}

func extractValidCoverProjects(projects []string) []string {
	validProjects := make([]string, 0, len(projects))
	seenProj := make(map[string]bool)
	for _, p := range projects {
		for _, subP := range splitProjectList(p) {
			cpClean := cleanProjectTitle(subP)
			if cpClean != "" && IsValidProjectName(cpClean) && !strings.EqualFold(cpClean, "General") && !seenProj[strings.ToLower(cpClean)] {
				validProjects = append(validProjects, cpClean)
				seenProj[strings.ToLower(cpClean)] = true
			}
		}
	}
	return validProjects
}

func cleanProjectTitle(s string) string {
	return cleanProjectName(s)
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func spacer() string {
	return `<w:p><w:pPr><w:spacing w:before="120" w:after="0"/></w:pPr></w:p>`
}

func pageBreak() string {
	return `<w:p><w:r><w:br w:type="page"/></w:r></w:p>`
}

func contentTypesXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Default Extension="jpg" ContentType="image/jpeg"/>
  <Default Extension="jpeg" ContentType="image/jpeg"/>
  <Default Extension="png" ContentType="image/png"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
  <Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/>
  <Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>
  <Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
  <Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>
  <Override PartName="/word/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>
  <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-officedocument.core-properties+xml"/>
  <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`
}

func relsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`
}

func mmToTwips(mm float64) int {
	return int(math.Round(mm * 56.6929))
}

func documentXML(body string, tmpl *models.Template) string {
	wTwips := mmToTwips(tmpl.PageConfig.WidthMM)
	hTwips := mmToTwips(tmpl.PageConfig.HeightMM)
	mTop := mmToTwips(tmpl.PageConfig.MarginTopMM)
	mBot := mmToTwips(tmpl.PageConfig.MarginBottomMM)
	mLeft := mmToTwips(tmpl.PageConfig.MarginLeftMM)
	mRight := mmToTwips(tmpl.PageConfig.MarginRightMM)

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:wpc="http://schemas.microsoft.com/office/word/2010/wordprocessingCanvas"
  xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"
  xmlns:o="urn:schemas-microsoft-com:office:office"
  xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
  xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"
  xmlns:v="urn:schemas-microsoft-com:vml"
  xmlns:wp14="http://schemas.microsoft.com/office/word/2010/wordprocessingDrawing"
  xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
  xmlns:w10="urn:schemas-microsoft-com:office:word"
  xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
  xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"
  xmlns:wpg="http://schemas.microsoft.com/office/word/2010/wordprocessingGroup"
  xmlns:wpi="http://schemas.microsoft.com/office/word/2010/wordprocessingInk"
  xmlns:wne="http://schemas.microsoft.com/office/word/2006/wordml"
  xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"
  mc:Ignorable="w14 wp14">
<w:body>
%s
<w:sectPr>
  <w:headerReference w:type="default" r:id="rId6"/>
  <w:footerReference w:type="default" r:id="rId7"/>
  <w:titlePg/>
  <w:pgSz w:w="%d" w:h="%d"/>
  <w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="540" w:footer="540" w:gutter="0"/>
</w:sectPr>
</w:body>
</w:document>`, body, wTwips, hTwips, mTop, mRight, mBot, mLeft)
}

func documentRels(hasCover bool, coverExt string, hasLogo bool, logoExt string) string {
	var extra strings.Builder
	nextID := 4
	if hasCover {
		extra.WriteString(fmt.Sprintf(`  <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/cover_hero.%s"/>`+"\n", nextID, coverExt))
		nextID++
	}
	if hasLogo {
		extra.WriteString(fmt.Sprintf(`  <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/team_logo.%s"/>`+"\n", nextID, logoExt))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml"/>
%s  <Relationship Id="rId6" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/>
  <Relationship Id="rId7" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>
</Relationships>`, extra.String())
}

func headerXML(tmpl *models.Template) string {
	if !tmpl.PageConfig.ShowPageNumbers {
		return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p/></w:hdr>`
	}
	primaryHex := hexColor(tmpl.PrimaryColor, "262262")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:p>
    <w:pPr><w:jc w:val="right"/><w:spacing w:before="0" w:after="0"/></w:pPr>
    <w:r>
      <w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="24"/></w:rPr>
      <w:fldSimple w:instr="PAGE"/>
    </w:r>
  </w:p>
</w:hdr>`, primaryHex)
}

func footerXML(tmpl *models.Template) string {
	footerText := strings.TrimSpace(tmpl.PageConfig.FooterText)
	if footerText == "" {
		return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p/></w:ftr>`
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:p>
    <w:pPr><w:jc w:val="left"/><w:spacing w:before="0" w:after="0"/></w:pPr>
    <w:r>
      <w:rPr><w:color w:val="555555"/><w:sz w:val="18"/></w:rPr>
      <w:t>%s</w:t>
    </w:r>
  </w:p>
</w:ftr>`, xmlEscape(footerText))
}

func stylesXML(tmpl *models.Template) string {
	descFont := tmpl.Typography.TaskDescription.FontFamily
	if descFont == "" {
		descFont = "Times New Roman"
	}
	descSz := ptToHalfPt(tmpl.Typography.TaskDescription.FontSizePt)
	lineVal := int(math.Round(tmpl.PageConfig.LineSpacing * 240.0))
	if lineVal < 240 {
		lineVal = 300
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:docDefaults>
    <w:rPrDefault>
      <w:rFonts w:ascii="%s" w:hAnsi="%s" w:cs="%s"/>
      <w:sz w:val="%d"/>
      <w:szCs w:val="%d"/>
    </w:rPrDefault>
  </w:docDefaults>
  <w:style w:type="paragraph" w:styleId="Normal">
    <w:name w:val="Normal"/>
    <w:pPr><w:spacing w:after="120" w:line="%d" w:lineRule="auto"/></w:pPr>
  </w:style>
</w:styles>`, xmlEscape(descFont), xmlEscape(descFont), xmlEscape(descFont), descSz, descSz, lineVal)
}

func settingsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:defaultTabStop w:val="720"/>
</w:settings>`
}

func themeXML(tmpl *models.Template) string {
	primaryHex := hexColor(tmpl.PrimaryColor, "262262")
	accentHex := hexColor(tmpl.AccentColor, "F5D061")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Office Theme">
  <a:themeElements>
    <a:clrScheme name="Office">
      <a:dk1><a:sysClr lastClr="000000" val="windowText"/></a:dk1>
      <a:lt1><a:sysClr lastClr="FFFFFF" val="window"/></a:lt1>
      <a:dk2><a:srgbClr val="1F2937"/></a:dk2>
      <a:lt2><a:srgbClr val="F9FAFB"/></a:lt2>
      <a:accent1><a:srgbClr val="%s"/></a:accent1>
      <a:accent2><a:srgbClr val="%s"/></a:accent2>
      <a:accent3><a:srgbClr val="009FE3"/></a:accent3>
    </a:clrScheme>
    <a:fontScheme name="Office">
      <a:majorFont><a:latin typeface="Century Gothic"/></a:majorFont>
      <a:minorFont><a:latin typeface="Times New Roman"/></a:minorFont>
    </a:fontScheme>
    <a:fmtScheme name="Office"/>
  </a:themeElements>
</a:theme>`, primaryHex, accentHex)
}

func coreXML(r *models.Report) string {
	t, _ := time.Parse("2006-01", r.Month)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
  xmlns:dc="http://purl.org/dc/elements/1.1/"
  xmlns:dcterms="http://purl.org/dc/terms/"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <dc:title>Monthly Performance Report - %s</dc:title>
  <dc:creator>%s</dc:creator>
  <dcterms:created xsi:type="dcterms:W3CDTF">%s</dcterms:created>
  <dcterms:modified xsi:type="dcterms:W3CDTF">%s</dcterms:modified>
</cp:coreProperties>`,
		t.Format("January 2006"),
		xmlEscape(r.CoverPage.EmployeeName),
		time.Now().Format(time.RFC3339),
		time.Now().Format(time.RFC3339),
	)
}

func appXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">
  <Application>Performance Report Generator</Application>
  <Company>MKCL</Company>
</Properties>`
}

// -----------------------------------------------------------------------------
// HTML Preview & Real Headless PDF Export
// -----------------------------------------------------------------------------

// GenerateHTMLPreview writes the print-ready paginated HTML file and returns its path.
func GenerateHTMLPreview(r *models.Report) (string, error) {
	exportsDir := storage.ExportsDir()
	if err := storage.EnsureDir(exportsDir); err != nil {
		return "", err
	}
	t, _ := time.Parse("2006-01", r.Month)
	filename := fmt.Sprintf("PerformanceReport%s.pdf.html", t.Format("January2006"))
	outputPath := filepath.Join(exportsDir, filename)

	content := buildPDFContent(r)
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return "", err
	}
	return outputPath, nil
}

// GeneratePDF creates a real PDF file (PerformanceReport<Month><Year>.pdf).
// It generates the completed DOCX first and converts it via Microsoft Word COM (or LibreOffice),
// falling back to headless Edge/Chrome HTML rendering if neither is available.
func GeneratePDF(r *models.Report) (string, error) {
	t, _ := time.Parse("2006-01", r.Month)
	pdfFilename := fmt.Sprintf("PerformanceReport%s.pdf", t.Format("January2006"))
	pdfPath := filepath.Join(storage.ExportsDir(), pdfFilename)

	if docxPath, err := GenerateDocx(r); err == nil && docxPath != "" {
		if convErr := ConvertDocxToPDF(docxPath, pdfPath); convErr == nil && storage.FileExists(pdfPath) {
			return pdfPath, nil
		}
	}

	htmlPath, err := GenerateHTMLPreview(r)
	if err != nil {
		return "", err
	}

	browserExe := findHeadlessBrowser()
	if browserExe == "" {
		return htmlPath, nil
	}

	absHTML, _ := filepath.Abs(htmlPath)
	absPDF, _ := filepath.Abs(pdfPath)
	fileURL := "file:///" + filepath.ToSlash(absHTML)
	userDataDir := filepath.Join(os.TempDir(), "pr-edge-pdf")
	_ = os.MkdirAll(userDataDir, 0755)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, browserExe,
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--user-data-dir="+userDataDir,
		"--no-pdf-header-footer",
		"--print-to-pdf="+absPDF,
		fileURL,
	)
	if _, err := cmd.CombinedOutput(); err != nil || !storage.FileExists(absPDF) {
		ctx2, cancel2 := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel2()
		cmd2 := exec.CommandContext(ctx2, browserExe,
			"--headless",
			"--disable-gpu",
			"--no-sandbox",
			"--user-data-dir="+userDataDir,
			"--no-pdf-header-footer",
			"--print-to-pdf="+absPDF,
			fileURL,
		)
		_, _ = cmd2.CombinedOutput()
	}

	if storage.FileExists(absPDF) {
		return absPDF, nil
	}
	return htmlPath, nil
}

func findHeadlessBrowser() string {
	candidates := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	}
	for _, c := range candidates {
		if storage.FileExists(c) {
			return c
		}
	}
	if p, err := exec.LookPath("msedge"); err == nil {
		return p
	}
	if p, err := exec.LookPath("chrome"); err == nil {
		return p
	}
	return ""
}

func BuildPDFContent(r *models.Report) string {
	return buildPDFContent(r)
}

func styleToCSS(ts models.TextStyle, fallbackFont string, fallbackPt float64, fallbackColor string) string {
	ff := ts.FontFamily
	if ff == "" {
		ff = fallbackFont
	}
	pt := ts.FontSizePt
	if pt <= 0 {
		pt = fallbackPt
	}
	col := ts.Color
	if col == "" {
		col = fallbackColor
	}
	fw := "normal"
	if ts.Bold {
		fw = "bold"
	}
	align := ts.Align
	if align == "" {
		align = "left"
	}
	return fmt.Sprintf("font-family: '%s', 'Times New Roman', sans-serif; font-size: %gpt; font-weight: %s; color: %s; text-align: %s;",
		ff, pt, fw, col, align)
}

func buildPDFContent(r *models.Report) string {
	cp := r.CoverPage
	tmpl := resolveEffectiveTemplate(r)
	paginated := paginateReport(r, tmpl)
	coverBytes, coverExt, logoBytes, logoExt := resolveTemplateImages(tmpl)

	coverDataURI := ""
	if len(coverBytes) > 0 {
		mime := "image/jpeg"
		if coverExt == "png" {
			mime = "image/png"
		}
		coverDataURI = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(coverBytes)
	}
	logoDataURI := ""
	if len(logoBytes) > 0 {
		mime := "image/png"
		if logoExt == "jpg" {
			mime = "image/jpeg"
		}
		logoDataURI = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(logoBytes)
	}

	isERA := tmpl.ID == "era" || tmpl.SourceType == "era-builtin"
	primaryColor := tmpl.PrimaryColor
	if primaryColor == "" {
		primaryColor = "#262262"
	}
	accentColor := tmpl.AccentColor
	if accentColor == "" {
		accentColor = "#F5D061"
	}

	var sb strings.Builder
	monthYearText := cp.MonthYear
	if monthYearText == "" {
		monthYearText = strings.TrimSpace(cp.Month + " " + cp.Year)
	}
	validProjects := extractValidCoverProjects(cp.Projects)
	projectLine := formatCoverProjectsLine(validProjects)

	pageW := tmpl.PageConfig.WidthMM
	pageH := tmpl.PageConfig.HeightMM
	mTop := tmpl.PageConfig.MarginTopMM
	mBot := tmpl.PageConfig.MarginBottomMM
	mLeft := tmpl.PageConfig.MarginLeftMM
	mRight := tmpl.PageConfig.MarginRightMM
	footerText := tmpl.PageConfig.FooterText

	sb.WriteString(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<title>Monthly Performance Report - ` + xmlEscape(monthYearText) + `</title>
<style>
  @page {
    size: ` + fmt.Sprintf("%.1fmm %.1fmm", pageW, pageH) + `;
    margin: 0;
  }
  * { box-sizing: border-box; }
  body {
    ` + styleToCSS(tmpl.Typography.TaskDescription, "Times New Roman", 14, "#222222") + `
    margin: 0;
    padding: 0;
    background: #f3f4f6;
    -webkit-print-color-adjust: exact;
    print-color-adjust: exact;
  }

  .sheet {
    width: ` + fmt.Sprintf("%.1fmm", pageW) + `;
    height: ` + fmt.Sprintf("%.1fmm", pageH) + `;
    min-height: ` + fmt.Sprintf("%.1fmm", pageH) + `;
    max-height: ` + fmt.Sprintf("%.1fmm", pageH) + `;
    padding: ` + fmt.Sprintf("%.1fmm %.1fmm %.1fmm %.1fmm", mTop, mRight, mBot, mLeft) + `;
    margin: 8mm auto;
    background: #ffffff;
    box-shadow: 0 4px 15px rgba(0,0,0,0.08);
    position: relative;
    overflow: hidden;
    page-break-after: always;
  }

  @media print {
    body { background: transparent; }
    .sheet {
      margin: 0;
      box-shadow: none;
      page-break-after: always;
    }
    .sheet:last-child {
      page-break-after: auto;
    }
  }

  .page-header-bar {
    position: absolute;
    top: 0;
    left: ` + fmt.Sprintf("%.1fmm", mLeft) + `;
    right: 0;
    height: 10mm;
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
  }
  .header-line {
    height: 4px;
    width: 60mm;
    background: ` + primaryColor + `;
    margin-top: 5mm;
  }
  .header-page-badge {
    background: ` + primaryColor + `;
    color: #ffffff;
    font-weight: bold;
    font-size: 14pt;
    padding: 2mm 6mm 3mm 6mm;
    clip-path: polygon(0 0, 100% 0, 100% 100%, 20% 100%);
    display: inline-block;
  }

  .public-tag {
    position: absolute;
    bottom: 8mm;
    left: 20mm;
    font-family: 'Century Gothic', sans-serif;
    font-size: 9pt;
    color: #555;
    letter-spacing: 0.5px;
  }

  .cover-sheet {
    padding: 0;
    position: relative;
    background: #ffffff;
  }
  .cover-yellow-backdrop {
    position: absolute;
    top: 16mm;
    left: 0;
    width: 175mm;
    height: 102mm;
    background: ` + accentColor + `;
    z-index: 1;
  }
  .cover-illustration-wrap {
    position: absolute;
    top: 22mm;
    left: 12mm;
    right: 12mm;
    z-index: 2;
    background: #ffffff;
    box-shadow: 0 4px 16px rgba(0,0,0,0.12);
    border-radius: 2px;
    overflow: hidden;
  }
  .cover-illustration-wrap img {
    width: 100%;
    height: auto;
    max-height: 92mm;
    object-fit: contain;
    display: block;
  }
  .cover-title-banner {
    position: absolute;
    top: 120mm;
    left: 0;
    width: 145mm;
    background: ` + primaryColor + `;
    color: #ffffff;
    padding: 6mm 14mm 6mm 18mm;
    z-index: 3;
    clip-path: polygon(0 0, 100% 0, 80% 100%, 0% 100%);
    -webkit-clip-path: polygon(0 0, 100% 0, 80% 100%, 0% 100%);
  }
  .cover-title-banner h1 {
    ` + styleToCSS(tmpl.Typography.BannerTitle, "Century Gothic", 28, "#FFFFFF") + `
    margin: 0;
    line-height: 1.12;
    letter-spacing: 0.5px;
  }
  .cover-title-banner .month-year {
    ` + styleToCSS(tmpl.Typography.BannerTitle, "Century Gothic", 28, "#FFFFFF") + `
    margin-top: 2.5mm;
    line-height: 1.12;
  }
  .cover-bottom-area {
    position: absolute;
    top: 174mm;
    left: 20mm;
    right: 20mm;
    display: flex;
    flex-direction: column;
    gap: 2.5mm;
    z-index: 3;
  }
  .cover-projects-line {
    ` + styleToCSS(tmpl.Typography.CoverProjects, "Century Gothic", 14, primaryColor) + `
    line-height: 1.35;
    word-break: break-word;
  }
  .cover-divider {
    width: 100%;
    border-bottom: 1.5px solid ` + primaryColor + `;
    margin: 1.5mm 0 3mm 0;
  }
  .cover-details-grid {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
  }
  .cover-emp-info {
    ` + styleToCSS(tmpl.Typography.CoverEmployee, "Avenir Next LT Pro Light", 14, "#111111") + `
    line-height: 1.6;
  }
  .cover-emp-info div {
    margin-bottom: 1mm;
  }
  .cover-emp-info strong {
    font-weight: 600;
  }
  .cover-logo-wrap {
    display: flex;
    align-items: flex-end;
    justify-content: flex-end;
  }
  .cover-logo-wrap img {
    width: 25mm;
    max-width: 25mm;
    height: auto;
    max-height: 16mm;
    object-fit: contain;
    display: block;
  }

  .team-cover-band {
    height: 22mm;
    background: ` + primaryColor + `;
    width: 100%;
  }
  .team-cover-body {
    padding: 25mm 22mm;
    display: flex;
    flex-direction: column;
    gap: 8mm;
  }
  .team-cover-title {
    ` + styleToCSS(tmpl.Typography.BannerTitle, "Calibri", 26, primaryColor) + `
    line-height: 1.2;
  }

  .toc-sheet {
    background: ` + accentColor + ` !important;
    padding: 26mm 24mm !important;
  }
  .toc-sheet-plain {
    background: #ffffff !important;
    padding: 26mm 24mm !important;
  }
  .toc-heading {
    ` + styleToCSS(tmpl.Typography.TOCTitle, "Century Gothic", 26, primaryColor) + `
    margin: 0 0 8mm 0;
    letter-spacing: 0.5px;
  }
  .toc-row-proj {
    display: block;
    margin-top: 6mm;
    margin-bottom: 2.5mm;
    ` + styleToCSS(tmpl.Typography.TOCProject, "Century Gothic", 16, "#333333") + `
  }
  .toc-row-task {
    display: block;
    margin-left: 7mm;
    margin-bottom: 2mm;
    line-height: 1.35;
    ` + styleToCSS(tmpl.Typography.TOCTask, "Times New Roman", 14, "#333333") + `
  }

  .project-header-title {
    ` + styleToCSS(tmpl.Typography.ProjectName, "Century Gothic", 23, primaryColor) + `
    width: 100%;
    margin: 5mm 0 4mm 0;
  }
  .task-main-title {
    ` + styleToCSS(tmpl.Typography.TaskTitle, "Times New Roman", 16, "#111111") + `
    width: 100%;
    margin-top: 3mm;
    margin-bottom: 1.8mm;
  }
  .sub-heading {
    ` + styleToCSS(tmpl.Typography.Subheading, "Times New Roman", 14, "#111111") + `
    width: 100%;
    margin-top: 2.2mm;
    margin-bottom: 1mm;
  }
  .bullet-row {
    ` + styleToCSS(tmpl.Typography.TaskDescription, "Times New Roman", 14, "#222222") + `
    width: 100%;
    line-height: ` + fmt.Sprintf("%.2f", tmpl.PageConfig.LineSpacing) + `;
    padding-left: ` + fmt.Sprintf("%.1fmm", tmpl.PageConfig.BulletIndentMM) + `;
    text-indent: -4.5mm;
    margin-bottom: 1.2mm;
  }
  .nested-bullet-row {
    ` + styleToCSS(tmpl.Typography.TaskDescription, "Times New Roman", 14, "#222222") + `
    width: 100%;
    line-height: ` + fmt.Sprintf("%.2f", tmpl.PageConfig.LineSpacing) + `;
    padding-left: ` + fmt.Sprintf("%.1fmm", tmpl.PageConfig.BulletIndentMM+6.0) + `;
    text-indent: -4.5mm;
    margin-bottom: 1.2mm;
  }
  .outcome-paragraph {
    ` + styleToCSS(tmpl.Typography.TaskDescription, "Times New Roman", 14, "#222222") + `
    width: 100%;
    line-height: ` + fmt.Sprintf("%.2f", tmpl.PageConfig.LineSpacing) + `;
    margin: 1.2mm 0 2.5mm 0;
  }
  .conclusion-title {
    ` + styleToCSS(tmpl.Typography.ConclusionTitle, "Century Gothic", 23, primaryColor) + `
    width: 100%;
    margin: 5mm 0 4mm 0;
  }
  .conclusion-p {
    ` + styleToCSS(tmpl.Typography.ConclusionBody, "Times New Roman", 14, "#222222") + `
    width: 100%;
    line-height: ` + fmt.Sprintf("%.2f", tmpl.PageConfig.LineSpacing) + `;
    margin-bottom: 3.5mm;
  }
</style>
</head>
<body>
`)

	if isERA {
		sb.WriteString(`  <div class="sheet cover-sheet">
    <div class="cover-yellow-backdrop"></div>`)
		if coverDataURI != "" {
			sb.WriteString(`
    <div class="cover-illustration-wrap">
      <img src="` + coverDataURI + `" alt="Cover Illustration" />
    </div>`)
		}
		sb.WriteString(`
    <div class="cover-title-banner">
      <h1>Monthly<br/>Performance Report</h1>
      <div class="month-year">` + xmlEscape(monthYearText) + `</div>
    </div>
    <div class="cover-bottom-area">`)
		if projectLine != "" {
			sb.WriteString(`
      <div class="cover-projects-line">Projects : ` + xmlEscape(projectLine) + `</div>`)
		}
		sb.WriteString(`
      <div class="cover-divider"></div>
      <div class="cover-details-grid">
        <div class="cover-emp-info">
          <div>Employee Name: <strong>` + xmlEscape(cp.EmployeeName) + `</strong></div>
          <div>Employee Code: <strong>` + xmlEscape(cp.EmployeeCode) + `</strong></div>
          <div>Appraiser Name: <strong>` + xmlEscape(cp.AppraiserName) + `</strong></div>
        </div>`)
		if logoDataURI != "" {
			sb.WriteString(`
        <div class="cover-logo-wrap"><img src="` + logoDataURI + `" alt="Logo" /></div>`)
		}
		sb.WriteString(`
      </div>
    </div>`)
		if footerText != "" {
			sb.WriteString(`<div class="public-tag">` + xmlEscape(footerText) + `</div>`)
		}
		sb.WriteString(`  </div>` + "\n")
	} else {
		sb.WriteString(`  <div class="sheet cover-sheet">
    <div class="team-cover-band"></div>
    <div class="team-cover-body">`)
		if logoDataURI != "" {
			sb.WriteString(`<div style="text-align:right;"><img src="` + logoDataURI + `" style="max-width:45mm;max-height:22mm;object-fit:contain;" alt="Team Logo"/></div>`)
		}
		if coverDataURI != "" {
			sb.WriteString(`<div style="text-align:center;"><img src="` + coverDataURI + `" style="max-width:160mm;max-height:80mm;object-fit:contain;border-radius:4px;" alt="Cover Graphic"/></div>`)
		}
		sb.WriteString(`
      <div class="team-cover-title">Monthly Performance Report<br/>` + xmlEscape(monthYearText) + `</div>`)
		if projectLine != "" {
			sb.WriteString(`<div class="cover-projects-line">Projects : ` + xmlEscape(projectLine) + `</div>`)
		}
		sb.WriteString(`
      <div class="cover-divider"></div>
      <div class="cover-emp-info">
        <div>Employee Name: <strong>` + xmlEscape(cp.EmployeeName) + `</strong></div>
        <div>Employee Code: <strong>` + xmlEscape(cp.EmployeeCode) + `</strong></div>
        <div>Appraiser Name: <strong>` + xmlEscape(cp.AppraiserName) + `</strong></div>
      </div>
    </div>`)
		if footerText != "" {
			sb.WriteString(`<div class="public-tag">` + xmlEscape(footerText) + `</div>`)
		}
		sb.WriteString(`  </div>` + "\n")
	}

	tocClass := "toc-sheet"
	if !isERA && tmpl.AccentColor == "#FFFFFF" {
		tocClass = "toc-sheet-plain"
	}
	sb.WriteString(`  <div class="sheet ` + tocClass + `">
    <h2 class="toc-heading">TABLE OF CONTENTS</h2>
`)
	for _, tocProj := range paginated.TOCProjects {
		sb.WriteString(fmt.Sprintf(`    <div class="toc-row-proj">▪ %s</div>`+"\n",
			xmlEscape(tocProj.Project)))
		for _, tk := range tocProj.Tasks {
			sb.WriteString(fmt.Sprintf(`    <div class="toc-row-task">• %s</div>`+"\n",
				xmlEscape(tk.Title)))
		}
	}
	if paginated.ConclusionPage > 0 {
		sb.WriteString(`    <div class="toc-row-proj">▪ Conclusion</div>` + "\n")
	}
	if footerText != "" {
		sb.WriteString(`    <div class="public-tag">` + xmlEscape(footerText) + `</div>` + "\n")
	}
	sb.WriteString(`  </div>` + "\n")

	for _, pg := range paginated.Pages {
		sb.WriteString(`  <div class="sheet">` + "\n")
		if tmpl.PageConfig.ShowPageNumbers {
			sb.WriteString(fmt.Sprintf(`    <div class="page-header-bar">
      <div class="header-line"></div>
      <div class="header-page-badge">%d</div>
    </div>`+"\n", pg.PageNumber))
		}
		for _, blk := range pg.Blocks {
			switch blk.Kind {
			case blockProjectHeader:
				sb.WriteString(`    <div class="project-header-title">` + xmlEscape(blk.Text) + `</div>` + "\n")
			case blockTaskTitle:
				sb.WriteString(`    <div class="task-main-title">• ` + xmlEscape(blk.Text) + `</div>` + "\n")
			case blockSubheading:
				sb.WriteString(`    <div class="sub-heading">` + xmlEscape(blk.Text) + `</div>` + "\n")
			case blockBullet:
				sb.WriteString(`    <div class="bullet-row">• ` + xmlEscape(blk.Text) + `</div>` + "\n")
			case blockNumberedItem:
				sb.WriteString(`    <div class="bullet-row">` + xmlEscape(blk.NumPrefix) + ` ` + xmlEscape(blk.Text) + `</div>` + "\n")
			case blockNestedBullet:
				sb.WriteString(`    <div class="nested-bullet-row">o ` + xmlEscape(blk.Text) + `</div>` + "\n")
			case blockParagraph:
				sb.WriteString(`    <div class="outcome-paragraph">` + xmlEscape(blk.Text) + `</div>` + "\n")
			case blockConclusionHead:
				sb.WriteString(`    <div class="conclusion-title">` + xmlEscape(blk.Text) + `</div>` + "\n")
			case blockConclusionPara:
				sb.WriteString(`    <div class="conclusion-p">` + xmlEscape(blk.Text) + `</div>` + "\n")
			}
		}
		if footerText != "" {
			sb.WriteString(`    <div class="public-tag">` + xmlEscape(footerText) + `</div>` + "\n")
		}
		sb.WriteString(`  </div>` + "\n")
	}

	sb.WriteString(`</body>
</html>
`)
	return sb.String()
}
