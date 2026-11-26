package services

import (
	"performancereport/models"
	"performancereport/storage"
)

func GetSettings() (*models.Settings, error) {
	var s models.Settings
	if err := storage.ReadJSON(storage.SettingsPath(), &s); err != nil {
		return nil, err
	}
	if s.DataPath == "" {
		s.DataPath = storage.DataPath()
	}
	return &s, nil
}

func SaveSettings(s *models.Settings) error {
	return storage.WriteJSON(storage.SettingsPath(), s)
}
