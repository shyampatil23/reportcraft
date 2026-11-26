package services

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"performancereport/models"
	"performancereport/storage"
)

// DefaultERATemplate returns the canonical MKCL ERA template configuration
// matching the uploaded 13-page reference PDF and exact 23pt/16pt/14pt rules.
func DefaultERATemplate() models.Template {
	now := time.Now().Format(time.RFC3339)
	return models.Template{
		ID:             "era",
		Name:           "ERA Monthly Performance Report",
		Description:    "Exact MKCL ERA performance report format with original cover artwork, golden Table of Contents, Century Gothic 23pt project headers, and Times New Roman 16pt/14pt task formatting.",
		IsDefault:      true,
		SourceType:     "era-builtin",
		FormattingMode: "match_template",
		Approved:       true,
		SchemaVersion:  3,
		PrimaryColor:   "#262262",
		AccentColor:    "#F5D061",
		Sections:       []string{"Cover", "Table of Contents", "Detailed Tasks", "Conclusion"},
		PageConfig: models.TemplatePageConfig{
			Size:               "Letter",
			Orientation:        "portrait",
			WidthMM:            215.9,
			HeightMM:           279.4,
			MarginTopMM:        20,
			MarginBottomMM:     18,
			MarginLeftMM:       20,
			MarginRightMM:      20,
			LineSpacing:        1.45,
			ParagraphSpacingPt: 6,
			BulletIndentMM:     8,
			ShowPageNumbers:    true,
			PageNumberStyle:    "badge",
			FooterText:         "Public",
		},
		Typography: models.TemplateTypography{
			BannerTitle:     models.TextStyle{FontFamily: "Century Gothic", FontSizePt: 28, Bold: true, Color: "#FFFFFF", Align: "left"},
			CoverProjects:   models.TextStyle{FontFamily: "Century Gothic", FontSizePt: 14, Bold: true, Color: "#262262", Align: "left"},
			CoverEmployee:   models.TextStyle{FontFamily: "Avenir Next LT Pro Light", FontSizePt: 14, Bold: false, Color: "#111111", Align: "left"},
			TOCTitle:        models.TextStyle{FontFamily: "Century Gothic", FontSizePt: 26, Bold: true, Color: "#262262", Align: "left"},
			TOCProject:      models.TextStyle{FontFamily: "Century Gothic", FontSizePt: 16, Bold: true, Color: "#333333", Align: "left"},
			TOCTask:         models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 14, Bold: false, Color: "#333333", Align: "left"},
			ProjectName:     models.TextStyle{FontFamily: "Century Gothic", FontSizePt: 23, Bold: true, Color: "#262262", Align: "left"},
			TaskTitle:       models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 16, Bold: true, Color: "#111111", Align: "left"},
			Subheading:      models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 14, Bold: true, Color: "#111111", Align: "left"},
			TaskDescription: models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 14, Bold: false, Color: "#222222", Align: "left"},
			ConclusionTitle: models.TextStyle{FontFamily: "Century Gothic", FontSizePt: 23, Bold: true, Color: "#262262", Align: "left"},
			ConclusionBody:  models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 14, Bold: false, Color: "#222222", Align: "justify"},
		},
		Assets: models.TemplateAssets{
			CoverHeroImage: "cover_illustration.jpg",
			LogoImage:      "mkcl_era_logo.png",
			ExtractedFiles: []string{"cover_illustration.jpg", "mkcl_era_logo.png"},
		},
		FieldMappings: []models.TemplateFieldMapping{
			{ID: "era-cover", FieldType: "cover_page", Label: "Cover Page", PageNumber: 1, X: 0, Y: 0, Width: 100, Height: 100, FontFamily: "Century Gothic", FontSizePt: 28, Bold: true, Color: "#262262"},
			{ID: "era-emp-name", FieldType: "employee_name", Label: "Employee Name", PageNumber: 1, X: 10, Y: 72, Width: 55, Height: 5, FontFamily: "Avenir Next LT Pro Light", FontSizePt: 14, Color: "#111111"},
			{ID: "era-emp-code", FieldType: "employee_code", Label: "Employee Code", PageNumber: 1, X: 10, Y: 77, Width: 55, Height: 5, FontFamily: "Avenir Next LT Pro Light", FontSizePt: 14, Color: "#111111"},
			{ID: "era-appraiser", FieldType: "appraiser_name", Label: "Appraiser Name", PageNumber: 1, X: 10, Y: 82, Width: 55, Height: 5, FontFamily: "Avenir Next LT Pro Light", FontSizePt: 14, Color: "#111111"},
			{ID: "era-projects", FieldType: "projects", Label: "Projects List", PageNumber: 1, X: 10, Y: 62, Width: 80, Height: 6, FontFamily: "Century Gothic", FontSizePt: 14, Bold: true, Color: "#262262"},
			{ID: "era-toc", FieldType: "toc_area", Label: "Table of Contents", PageNumber: 2, X: 10, Y: 10, Width: 80, Height: 80, FontFamily: "Century Gothic", FontSizePt: 16, Color: "#262262"},
			{ID: "era-task", FieldType: "repeatable_task", Label: "Repeatable Project & Task Section", PageNumber: 3, X: 11, Y: 10, Width: 78, Height: 80, FontFamily: "Times New Roman", FontSizePt: 14, Color: "#222222"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// ensureERATemplateInitialized guarantees the built-in ERA template and its assets exist on disk.
func ensureERATemplateInitialized() {
	eraDir := storage.TemplateDir("era")
	assetsDir := storage.TemplateAssetsDir("era")
	_ = storage.EnsureDir(assetsDir)

	coverPath := filepath.Join(assetsDir, "cover_illustration.jpg")
	if !storage.FileExists(coverPath) && CoverIllustrationBase64 != "" {
		if raw, err := base64.StdEncoding.DecodeString(CoverIllustrationBase64); err == nil {
			_ = os.WriteFile(coverPath, raw, 0644)
		}
	}
	logoPath := filepath.Join(assetsDir, "mkcl_era_logo.png")
	if !storage.FileExists(logoPath) && LogoBase64 != "" {
		if raw, err := base64.StdEncoding.DecodeString(LogoBase64); err == nil {
			_ = os.WriteFile(logoPath, raw, 0644)
		}
	}

	jsonPath := storage.TemplateJSONPath("era")
	if !storage.FileExists(jsonPath) {
		era := DefaultERATemplate()
		_ = storage.WriteJSON(jsonPath, era)
	} else {
		var existing models.Template
		if err := storage.ReadJSON(jsonPath, &existing); err == nil && existing.SchemaVersion < 3 {
			era := DefaultERATemplate()
			_ = storage.WriteJSON(jsonPath, era)
		}
	}
	_ = eraDir
}

// GetTemplates returns all locally persisted templates from data/templates/<id>/template.json.
func GetTemplates() ([]models.Template, error) {
	ensureERATemplateInitialized()
	templatesDir := storage.TemplatesDir()
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []models.Template{DefaultERATemplate()}, nil
		}
		return nil, err
	}

	var list []models.Template
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var tmpl models.Template
		tPath := storage.TemplateJSONPath(entry.Name())
		if !storage.FileExists(tPath) {
			continue
		}
		if err := storage.ReadJSON(tPath, &tmpl); err == nil && tmpl.ID != "" {
			list = append(list, tmpl)
		}
	}

	// Migrate legacy templates.json entries if any exist and weren't migrated yet
	if len(list) == 0 {
		era := DefaultERATemplate()
		_ = storage.WriteJSON(storage.TemplateJSONPath("era"), era)
		list = append(list, era)
	}

	// Ensure at least one default
	hasDefault := false
	for _, t := range list {
		if t.IsDefault {
			hasDefault = true
			break
		}
	}
	if !hasDefault && len(list) > 0 {
		list[0].IsDefault = true
		_ = storage.WriteJSON(storage.TemplateJSONPath(list[0].ID), list[0])
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].ID == "era" {
			return true
		}
		if list[j].ID == "era" {
			return false
		}
		return list[i].CreatedAt < list[j].CreatedAt
	})

	return list, nil
}

// GetTemplate retrieves a single template by ID, or returns the default template if id == "".
func GetTemplate(id string) (*models.Template, error) {
	templates, err := GetTemplates()
	if err != nil {
		return nil, err
	}
	if id == "" {
		for i := range templates {
			if templates[i].IsDefault {
				return &templates[i], nil
			}
		}
		if len(templates) > 0 {
			return &templates[0], nil
		}
		era := DefaultERATemplate()
		return &era, nil
	}
	for i := range templates {
		if templates[i].ID == id || (id == "mkcl-era-default" && templates[i].ID == "era") {
			return &templates[i], nil
		}
	}
	return nil, fmt.Errorf("template %q not found", id)
}

// SaveTemplate persists updates to a template's template.json.
func SaveTemplate(t *models.Template) error {
	if t.ID == "" {
		return fmt.Errorf("template ID is required")
	}
	t.UpdatedAt = time.Now().Format(time.RFC3339)
	if t.SchemaVersion == 0 {
		t.SchemaVersion = 2
	}
	if err := storage.EnsureDir(storage.TemplateAssetsDir(t.ID)); err != nil {
		return err
	}
	return storage.WriteJSON(storage.TemplateJSONPath(t.ID), t)
}

// SetDefaultTemplate marks one template as default and unmarks all others.
func SetDefaultTemplate(id string) (*models.Template, error) {
	templates, err := GetTemplates()
	if err != nil {
		return nil, err
	}
	var target *models.Template
	for i := range templates {
		templates[i].IsDefault = (templates[i].ID == id)
		if templates[i].ID == id {
			tCopy := templates[i]
			target = &tCopy
		}
		_ = SaveTemplate(&templates[i])
	}
	if target == nil {
		return nil, fmt.Errorf("template %q not found", id)
	}
	return target, nil
}

// DuplicateTemplate clones an existing template and its isolated assets folder.
func DuplicateTemplate(id string) (*models.Template, error) {
	orig, err := GetTemplate(id)
	if err != nil {
		return nil, err
	}
	newID := sanitizeSlug(orig.Name) + "-" + uuid.New().String()[:6]
	now := time.Now().Format(time.RFC3339)
	clone := *orig
	clone.ID = newID
	clone.Name = orig.Name + " (Copy)"
	clone.IsDefault = false
	clone.CreatedAt = now
	clone.UpdatedAt = now
	if clone.SourceType == "era-builtin" {
		// Cloned ERA template becomes a standalone editable template with its own assets
		clone.SourceType = "custom"
	}

	if err := storage.EnsureDir(storage.TemplateAssetsDir(newID)); err != nil {
		return nil, err
	}
	// Copy source file if present
	if orig.SourceFile != "" {
		srcPath := filepath.Join(storage.TemplateDir(orig.ID), orig.SourceFile)
		dstPath := filepath.Join(storage.TemplateDir(newID), orig.SourceFile)
		if storage.FileExists(srcPath) {
			_ = storage.CopyFile(srcPath, dstPath)
		}
	}
	// Copy assets
	origAssetsDir := storage.TemplateAssetsDir(orig.ID)
	if entries, err := os.ReadDir(origAssetsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				_ = storage.CopyFile(
					filepath.Join(origAssetsDir, e.Name()),
					filepath.Join(storage.TemplateAssetsDir(newID), e.Name()),
				)
			}
		}
	}

	if err := SaveTemplate(&clone); err != nil {
		return nil, err
	}
	return &clone, nil
}

// DeleteTemplate deletes a custom template and its directory. Built-in 'era' cannot be deleted.
func DeleteTemplate(id string) error {
	if id == "era" || id == "mkcl-era-default" {
		return fmt.Errorf("cannot delete the built-in ERA default template")
	}
	tmpl, err := GetTemplate(id)
	if err != nil {
		return err
	}
	wasDefault := tmpl.IsDefault
	if err := os.RemoveAll(storage.TemplateDir(id)); err != nil {
		return err
	}
	if wasDefault {
		_, _ = SetDefaultTemplate("era")
	}
	return nil
}

// LoadTemplateAssetBase64 reads an asset strictly from the given template's isolated assets directory.
func LoadTemplateAssetBase64(templateID, filename string) string {
	if templateID == "" || filename == "" {
		return ""
	}
	p := filepath.Join(storage.TemplateAssetsDir(templateID), filepath.Base(filename))
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

// ImportTemplateFile saves an uploaded PDF or DOCX file, extracts isolated assets,
// performs automatic layout/typography analysis, and creates template.json.
// It NEVER applies ERA images or ERA formatting to other teams' templates.
func ImportTemplateFile(name, primaryColor, accentColor, origFilename string, fileBytes []byte) (*models.Template, error) {
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSuffix(origFilename, filepath.Ext(origFilename))
	}
	ext := strings.ToLower(filepath.Ext(origFilename))
	if ext != ".pdf" && ext != ".docx" {
		return nil, fmt.Errorf("unsupported file type %q: please upload a .pdf or .docx template", ext)
	}
	if primaryColor == "" {
		primaryColor = "#1E293B"
	}
	if accentColor == "" {
		accentColor = "#E2E8F0"
	}

	id := sanitizeSlug(name) + "-" + uuid.New().String()[:6]
	tmplDir := storage.TemplateDir(id)
	assetsDir := storage.TemplateAssetsDir(id)
	if err := storage.EnsureDir(assetsDir); err != nil {
		return nil, err
	}

	sourceFilename := "source" + ext
	sourcePath := filepath.Join(tmplDir, sourceFilename)
	if err := os.WriteFile(sourcePath, fileBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to save template source file: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	tmpl := models.Template{
		ID:             id,
		Name:           strings.TrimSpace(name),
		Description:    fmt.Sprintf("Imported from %s. Preserves team-specific layout, fonts, and extracted visual assets.", origFilename),
		IsDefault:      false,
		SourceType:     strings.TrimPrefix(ext, "."),
		SourceFile:     sourceFilename,
		FormattingMode: "match_template",
		Approved:       false, // Requires user verification in Mapping Wizard / Compare view
		SchemaVersion:  2,
		PrimaryColor:   primaryColor,
		AccentColor:    accentColor,
		Sections:       []string{"Cover", "Table of Contents", "Detailed Tasks", "Conclusion"},
		PageConfig: models.TemplatePageConfig{
			Size:               "A4",
			Orientation:        "portrait",
			WidthMM:            210,
			HeightMM:           297,
			MarginTopMM:        20,
			MarginBottomMM:     20,
			MarginLeftMM:       20,
			MarginRightMM:      20,
			LineSpacing:        1.45,
			ParagraphSpacingPt: 6,
			BulletIndentMM:     6,
			ShowPageNumbers:    true,
			PageNumberStyle:    "plain",
			FooterText:         "",
		},
		Typography: models.TemplateTypography{
			BannerTitle:     models.TextStyle{FontFamily: "Calibri", FontSizePt: 24, Bold: true, Color: primaryColor, Align: "left"},
			CoverProjects:   models.TextStyle{FontFamily: "Calibri", FontSizePt: 14, Bold: true, Color: primaryColor, Align: "left"},
			CoverEmployee:   models.TextStyle{FontFamily: "Calibri", FontSizePt: 12, Bold: false, Color: "#111111", Align: "left"},
			TOCTitle:        models.TextStyle{FontFamily: "Calibri", FontSizePt: 22, Bold: true, Color: primaryColor, Align: "left"},
			TOCProject:      models.TextStyle{FontFamily: "Calibri", FontSizePt: 14, Bold: true, Color: "#222222", Align: "left"},
			TOCTask:         models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 12, Bold: false, Color: "#333333", Align: "left"},
			ProjectName:     models.TextStyle{FontFamily: "Century Gothic", FontSizePt: 23, Bold: true, Color: primaryColor, Align: "left"},
			TaskTitle:       models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 16, Bold: true, Color: "#111111", Align: "left"},
			Subheading:      models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 14, Bold: true, Color: "#111111", Align: "left"},
			TaskDescription: models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 14, Bold: false, Color: "#222222", Align: "left"},
			ConclusionTitle: models.TextStyle{FontFamily: "Century Gothic", FontSizePt: 23, Bold: true, Color: primaryColor, Align: "left"},
			ConclusionBody:  models.TextStyle{FontFamily: "Times New Roman", FontSizePt: 14, Bold: false, Color: "#222222", Align: "justify"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if ext == ".docx" {
		analyzeDocxTemplate(sourcePath, assetsDir, &tmpl)
	} else {
		analyzePDFTemplate(sourcePath, assetsDir, &tmpl)
	}

	if err := SaveTemplate(&tmpl); err != nil {
		return nil, err
	}
	return &tmpl, nil
}

func analyzeDocxTemplate(docxPath, assetsDir string, tmpl *models.Template) {
	analysis := &models.TemplateAnalysisResult{
		PageWidthPt:  595.28,
		PageHeightPt: 841.89,
		Orientation:  "portrait",
	}

	r, err := zip.OpenReader(docxPath)
	if err != nil {
		analysis.Notes = append(analysis.Notes, "Could not open DOCX archive: "+err.Error())
		tmpl.DetectedInfo = analysis
		return
	}
	defer r.Close()

	fontSet := map[string]bool{}
	sizeSet := map[float64]bool{}
	colorSet := map[string]bool{}
	var extracted []string

	for _, f := range r.File {
		// Extract embedded images from word/media/*
		if strings.HasPrefix(f.Name, "word/media/") && !f.FileInfo().IsDir() {
			imgName := filepath.Base(f.Name)
			rc, err := f.Open()
			if err == nil {
				data, err := io.ReadAll(rc)
				rc.Close()
				if err == nil && len(data) > 0 {
					outPath := filepath.Join(assetsDir, imgName)
					if os.WriteFile(outPath, data, 0644) == nil {
						extracted = append(extracted, imgName)
					}
				}
			}
			continue
		}

		if f.Name == "word/document.xml" || f.Name == "word/styles.xml" {
			rc, err := f.Open()
			if err == nil {
				xmlBytes, _ := io.ReadAll(rc)
				rc.Close()
				parseDocxXMLMetadata(xmlBytes, f.Name == "word/document.xml", analysis, fontSet, sizeSet, colorSet)
			}
		}
	}

	for fn := range fontSet {
		analysis.Fonts = append(analysis.Fonts, fn)
	}
	sort.Strings(analysis.Fonts)
	for sz := range sizeSet {
		analysis.FontSizesPt = append(analysis.FontSizesPt, sz)
	}
	sort.Float64s(analysis.FontSizesPt)
	for c := range colorSet {
		analysis.Colors = append(analysis.Colors, c)
	}
	sort.Strings(analysis.Colors)

	analysis.ImagesCount = len(extracted)
	tmpl.Assets.ExtractedFiles = extracted
	if len(extracted) > 0 {
		tmpl.Assets.LogoImage = extracted[0]
		if len(extracted) > 1 {
			tmpl.Assets.CoverHeroImage = extracted[0]
			tmpl.Assets.LogoImage = extracted[1]
		}
	}

	// Apply detected page dimensions to PageConfig (1 pt = 0.352778 mm)
	if analysis.PageWidthPt > 0 && analysis.PageHeightPt > 0 {
		tmpl.PageConfig.WidthMM = round1(analysis.PageWidthPt * 0.352778)
		tmpl.PageConfig.HeightMM = round1(analysis.PageHeightPt * 0.352778)
		tmpl.PageConfig.Orientation = analysis.Orientation
		if analysis.MarginTopPt > 0 {
			tmpl.PageConfig.MarginTopMM = round1(analysis.MarginTopPt * 0.352778)
		}
		if analysis.MarginBottomPt > 0 {
			tmpl.PageConfig.MarginBottomMM = round1(analysis.MarginBottomPt * 0.352778)
		}
		if analysis.MarginLeftPt > 0 {
			tmpl.PageConfig.MarginLeftMM = round1(analysis.MarginLeftPt * 0.352778)
		}
		if analysis.MarginRightPt > 0 {
			tmpl.PageConfig.MarginRightMM = round1(analysis.MarginRightPt * 0.352778)
		}
	}

	if len(analysis.Fonts) > 0 {
		primaryFont := analysis.Fonts[0]
		tmpl.Typography.BannerTitle.FontFamily = primaryFont
		tmpl.Typography.ProjectName.FontFamily = primaryFont
		tmpl.Typography.TaskDescription.FontFamily = primaryFont
	}
	if len(analysis.Colors) > 0 {
		tmpl.PrimaryColor = analysis.Colors[0]
		tmpl.Typography.ProjectName.Color = analysis.Colors[0]
		tmpl.Typography.BannerTitle.Color = analysis.Colors[0]
	}

	analysis.Notes = append(analysis.Notes,
		fmt.Sprintf("Extracted %d image asset(s) from DOCX media folder.", len(extracted)),
		"DOCX native layout and styles detected; open Mapping Wizard to bind or refine fields.",
	)
	tmpl.DetectedInfo = analysis
}

func parseDocxXMLMetadata(data []byte, isDoc bool, analysis *models.TemplateAnalysisResult, fonts map[string]bool, sizes map[float64]bool, colors map[string]bool) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "pgSz":
			if isDoc {
				for _, attr := range se.Attr {
					v, _ := strconv.ParseFloat(attr.Value, 64)
					if attr.Name.Local == "w" && v > 0 {
						analysis.PageWidthPt = round1(v / 20.0)
					} else if attr.Name.Local == "h" && v > 0 {
						analysis.PageHeightPt = round1(v / 20.0)
					} else if attr.Name.Local == "orient" && attr.Value == "landscape" {
						analysis.Orientation = "landscape"
					}
				}
			}
		case "pgMar":
			if isDoc {
				for _, attr := range se.Attr {
					v, _ := strconv.ParseFloat(attr.Value, 64)
					pt := round1(v / 20.0)
					switch attr.Name.Local {
					case "top":
						analysis.MarginTopPt = pt
					case "bottom":
						analysis.MarginBottomPt = pt
					case "left":
						analysis.MarginLeftPt = pt
					case "right":
						analysis.MarginRightPt = pt
					}
				}
			}
		case "rFonts":
			for _, attr := range se.Attr {
				if (attr.Name.Local == "ascii" || attr.Name.Local == "hAnsi") && attr.Value != "" {
					fonts[attr.Value] = true
				}
			}
		case "sz":
			for _, attr := range se.Attr {
				if attr.Name.Local == "val" {
					if v, err := strconv.ParseFloat(attr.Value, 64); err == nil && v > 0 {
						sizes[v/2.0] = true
					}
				}
			}
		case "color":
			for _, attr := range se.Attr {
				if attr.Name.Local == "val" && attr.Value != "" && attr.Value != "auto" && attr.Value != "FFFFFF" && attr.Value != "000000" {
					colors["#"+strings.ToUpper(attr.Value)] = true
				}
			}
		case "t":
			if isDoc {
				var txt string
				if err := dec.DecodeElement(&txt, &se); err == nil {
					low := strings.ToLower(strings.TrimSpace(txt))
					if strings.Contains(low, "table of contents") || strings.Contains(low, "contents") {
						analysis.HasTOC = true
					}
				}
			}
		}
	}
}

func analyzePDFTemplate(pdfPath, assetsDir string, tmpl *models.Template) {
	analysis := &models.TemplateAnalysisResult{
		Orientation: "portrait",
	}

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

	bgCtx := context.Background()
	ctx, err := api.ReadContextFile(bgCtx, pdfPath)
	if err == nil {
		analysis.PageCount = ctx.PageCount
		if dims, err := api.PageDimsFile(bgCtx, pdfPath); err == nil && len(dims) > 0 {
			analysis.PageWidthPt = round1(dims[0].Width)
			analysis.PageHeightPt = round1(dims[0].Height)
			if dims[0].Width > dims[0].Height {
				analysis.Orientation = "landscape"
			}
			tmpl.PageConfig.WidthMM = round1(dims[0].Width * 0.352778)
			tmpl.PageConfig.HeightMM = round1(dims[0].Height * 0.352778)
			tmpl.PageConfig.Orientation = analysis.Orientation
		}
	} else {
		analysis.Notes = append(analysis.Notes, "PDF metadata inspection note: "+err.Error())
	}

	// Extract embedded images into template-specific assets folder (never flattening whole pages!)
	_ = api.ExtractImagesFile(bgCtx, pdfPath, assetsDir, nil, conf)

	var extracted []string
	if entries, err := os.ReadDir(assetsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				extracted = append(extracted, e.Name())
			}
		}
	}
	sort.Strings(extracted)
	analysis.ImagesCount = len(extracted)
	tmpl.Assets.ExtractedFiles = extracted
	if len(extracted) > 0 {
		tmpl.Assets.CoverHeroImage = extracted[0]
		if len(extracted) > 1 {
			tmpl.Assets.LogoImage = extracted[1]
		}
	}

	analysis.Notes = append(analysis.Notes,
		fmt.Sprintf("Detected %d page(s) (%0.1f x %0.1f pt, %s).", analysis.PageCount, analysis.PageWidthPt, analysis.PageHeightPt, analysis.Orientation),
		fmt.Sprintf("Extracted %d embedded image asset(s) into isolated template storage.", len(extracted)),
		"Arbitrary PDFs require field mapping and visual verification in the Mapping Wizard before approval.",
	)
	tmpl.DetectedInfo = analysis
}

func sanitizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteRune('-')
		}
	}
	res := strings.Trim(b.String(), "-")
	if res == "" {
		return "template"
	}
	if len(res) > 24 {
		res = res[:24]
	}
	return res
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10.0
}
