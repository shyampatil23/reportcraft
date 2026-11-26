package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"performancereport/storage"
)

// ConvertDocxToPDF converts a completed .docx file to .pdf using Microsoft Word COM automation
// (wdExportFormatPDF = 17) on Windows, with LibreOffice headless as a secondary fallback.
func ConvertDocxToPDF(docxPath, pdfPath string) error {
	absDocx, err := filepath.Abs(docxPath)
	if err != nil {
		return err
	}
	absPDF, err := filepath.Abs(pdfPath)
	if err != nil {
		return err
	}
	if !storage.FileExists(absDocx) {
		return fmt.Errorf("source docx does not exist: %s", absDocx)
	}
	_ = os.Remove(absPDF)

	if err := convertWithWordCOM(absDocx, absPDF); err == nil && storage.FileExists(absPDF) {
		return nil
	} else if soffice, lookErr := exec.LookPath("soffice"); lookErr == nil && soffice != "" {
		if loErr := convertWithLibreOffice(soffice, absDocx, absPDF); loErr == nil && storage.FileExists(absPDF) {
			return nil
		}
		return err
	} else {
		return err
	}
}

func convertWithWordCOM(absDocx, absPDF string) error {
	// Escape single quotes for PowerShell single-quoted strings
	psDocx := strings.ReplaceAll(absDocx, "'", "''")
	psPDF := strings.ReplaceAll(absPDF, "'", "''")

	psScript := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$word = $null
$doc = $null
try {
    $word = New-Object -ComObject Word.Application
    $word.Visible = $false
    $word.DisplayAlerts = 0
    $doc = $word.Documents.Open('%s', $false, $true)
    $doc.ExportAsFixedFormat('%s', 17)
} finally {
    if ($doc -ne $null) {
        try { $doc.Close($false) } catch {}
        try { [System.Runtime.Interopservices.Marshal]::ReleaseComObject($doc) | Out-Null } catch {}
    }
    if ($word -ne $null) {
        try { $word.Quit($false) } catch {}
        try { [System.Runtime.Interopservices.Marshal]::ReleaseComObject($word) | Out-Null } catch {}
    }
}
`, psDocx, psPDF)

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("word COM export failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if !storage.FileExists(absPDF) {
		return fmt.Errorf("word COM export produced no PDF at %s", absPDF)
	}
	return nil
}

func convertWithLibreOffice(soffice, absDocx, absPDF string) error {
	outDir := filepath.Dir(absPDF)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, soffice, "--headless", "--convert-to", "pdf", "--outdir", outDir, absDocx)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("libreoffice export failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	expected := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(absDocx), filepath.Ext(absDocx))+".pdf")
	if expected != absPDF && storage.FileExists(expected) {
		_ = os.Rename(expected, absPDF)
	}
	if !storage.FileExists(absPDF) {
		return fmt.Errorf("libreoffice export produced no PDF at %s", absPDF)
	}
	return nil
}
