package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var mu sync.RWMutex

// DataPath returns the base data directory
func DataPath() string {
	if custom := os.Getenv("PERFORMANCE_REPORT_DATA_DIR"); custom != "" {
		return custom
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, _ := os.UserHomeDir()
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(localAppData, "PerformanceReportBuilder", "data")
}

// EnsureDir creates a directory (and parents) if it does not exist
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// ReadJSON reads a JSON file into v safely, backing up corrupted files
func ReadJSON(path string, v interface{}) error {
	mu.RLock()
	f, err := os.Open(path)
	if err != nil {
		mu.RUnlock()
		if os.IsNotExist(err) {
			return nil // file not yet created is fine
		}
		return err
	}
	err = json.NewDecoder(f).Decode(v)
	f.Close()
	mu.RUnlock()
	if err != nil {
		_ = BackupFile(path)
		return fmt.Errorf("corrupted JSON in %s (backed up): %w", filepath.Base(path), err)
	}
	return nil
}

// WriteJSON writes v to a JSON file atomically (write temp, rename)
func WriteJSON(path string, v interface{}) error {
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, path)
}

// DeleteFile deletes a file if it exists
func DeleteFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(path)
}

// CopyFile copies src to dst
func CopyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// BackupFile creates a timestamped backup copy of a file
func BackupFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	dir := filepath.Dir(path)
	name := filepath.Base(path)
	ts := time.Now().Format("20060102-150405")
	dst := filepath.Join(dir, "backups", fmt.Sprintf("%s.%s.bak", name, ts))
	return CopyFile(path, dst)
}

// ListFiles returns a slice of file paths matching glob pattern
func ListFiles(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

// FileExists returns true if path exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// TasksPath returns the path to the tasks file for a given month
func TasksPath(month string) string {
	return filepath.Join(DataPath(), "tasks", month+".json")
}

// FullMonthDraftPath returns the path to the full-month draft for a given month
func FullMonthDraftPath(month string) string {
	return filepath.Join(DataPath(), "full-month-drafts", month+".json")
}

// ReportDraftPath returns the path to the report draft for a given month
func ReportDraftPath(month string) string {
	return filepath.Join(DataPath(), "reports", month, "draft.json")
}

// ReportStatusPath returns the lifecycle status.json path for a given month
func ReportStatusPath(month string) string {
	return filepath.Join(DataPath(), "reports", month, "status.json")
}

// ReportVersionPath returns the path for a report version snapshot
func ReportVersionPath(month string, version int) string {
	return filepath.Join(DataPath(), "reports", month, "versions", fmt.Sprintf("v%d.json", version))
}

// ProfilePath returns the path to the profile file
func ProfilePath() string {
	return filepath.Join(DataPath(), "profile.json")
}

// SettingsPath returns the path to the settings file
func SettingsPath() string {
	return filepath.Join(DataPath(), "settings.json")
}

// TemplatesPath returns the path to the templates index file
func TemplatesPath() string {
	return filepath.Join(DataPath(), "templates", "templates.json")
}

// TemplatesDir returns the root templates directory
func TemplatesDir() string {
	return filepath.Join(DataPath(), "templates")
}

// TemplateDir returns the directory for a specific template
func TemplateDir(id string) string {
	return filepath.Join(DataPath(), "templates", id)
}

// TemplateJSONPath returns the template.json path for a specific template
func TemplateJSONPath(id string) string {
	return filepath.Join(TemplateDir(id), "template.json")
}

// TemplateAssetsDir returns the assets directory for a specific template
func TemplateAssetsDir(id string) string {
	return filepath.Join(TemplateDir(id), "assets")
}

// AICredsPath returns the path to the AI credentials file
func AICredsPath() string {
	return filepath.Join(DataPath(), "credentials", "ai.json")
}

// GitLabCredsPath returns the path to the GitLab credentials file
func GitLabCredsPath() string {
	return filepath.Join(DataPath(), "credentials", "gitlab.json")
}

// BackupsDir returns the backups directory
func BackupsDir() string {
	return filepath.Join(DataPath(), "backups")
}

// ExportsDir returns the exports directory
func ExportsDir() string {
	return filepath.Join(DataPath(), "exports")
}

// ReportEditedDir returns the edited-document directory for a month
func ReportEditedDir(month string) string {
	return filepath.Join(DataPath(), "reports", month, "edited")
}

// ReportEditorDraftPath returns the in-progress editor draft path
func ReportEditorDraftPath(month string) string {
	return filepath.Join(ReportEditedDir(month), "draft_editor.json")
}

// ReportEditorSavedPath returns the permanently saved editor document path
func ReportEditorSavedPath(month string) string {
	return filepath.Join(ReportEditedDir(month), "saved_editor.json")
}

// ReportEditedDocxPath returns the latest saved edited DOCX path
func ReportEditedDocxPath(month string) string {
	return filepath.Join(ReportEditedDir(month), "edited_report.docx")
}

// ReportEditedPDFPath returns the latest saved edited PDF path
func ReportEditedPDFPath(month string) string {
	return filepath.Join(ReportEditedDir(month), "edited_report.pdf")
}

// ReportWordWorkingCopyPath returns the working DOCX copy opened in Microsoft Word
func ReportWordWorkingCopyPath(month string) string {
	return filepath.Join(ReportEditedDir(month), "working_copy.docx")
}
