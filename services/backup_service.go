package services

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"performancereport/models"
	"performancereport/storage"
)

// CreateBackup zips the entire data directory into a backup file.
func CreateBackup() (*models.BackupInfo, error) {
	backupsDir := storage.BackupsDir()
	if err := storage.EnsureDir(backupsDir); err != nil {
		return nil, err
	}

	ts := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("PerformanceReport-Backup-%s.zip", ts)
	destPath := filepath.Join(backupsDir, filename)

	dataDir := storage.DataPath()
	if err := zipDir(dataDir, destPath, backupsDir); err != nil {
		return nil, err
	}

	fi, err := os.Stat(destPath)
	if err != nil {
		return nil, err
	}

	return &models.BackupInfo{
		Filename:  filename,
		CreatedAt: time.Now().Format(time.RFC3339),
		SizeMB:    float64(fi.Size()) / 1024 / 1024,
	}, nil
}

// ListBackups returns all backup files.
func ListBackups() ([]models.BackupInfo, error) {
	backupsDir := storage.BackupsDir()
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var backups []models.BackupInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		info, _ := e.Info()
		backups = append(backups, models.BackupInfo{
			Filename:  e.Name(),
			CreatedAt: info.ModTime().Format(time.RFC3339),
			SizeMB:    float64(info.Size()) / 1024 / 1024,
		})
	}
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt > backups[j].CreatedAt
	})
	return backups, nil
}

// RestoreBackup restores data from a backup zip file.
func RestoreBackup(filename string) error {
	backupPath := filepath.Join(storage.BackupsDir(), filename)
	dataDir := storage.DataPath()
	return unzipDir(backupPath, dataDir)
}

func zipDir(source, dest, exclude string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip backups folder itself
		if strings.HasPrefix(path, exclude) && path != source {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		fw, err := w.Create(rel)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(fw, file)
		return err
	})
}

func unzipDir(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		destPath := filepath.Join(dest, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, 0755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}
		out, err := os.Create(destPath)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
