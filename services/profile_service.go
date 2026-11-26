package services

import (
	"performancereport/models"
	"performancereport/storage"
)

func GetProfile() (*models.Profile, error) {
	var p models.Profile
	if err := storage.ReadJSON(storage.ProfilePath(), &p); err != nil {
		return nil, err
	}
	if p.EmployeeName == "" || p.EmployeeName == "Sham Patil" {
		p.EmployeeName = "Sham Anil Patil"
		p.EmployeeCode = "1947"
		p.AppraiserName = "Shivprakash Bapat"
		if p.Department == "" {
			p.Department = "ERA"
		}
	}
	if len(p.Projects) == 0 {
		p.Projects = []string{
			"ERA Live 2.0",
			"ERA Live 3.0",
			"EMACloudContentManager",
			"EMASECUREBROWSER",
		}
	}
	return &p, nil
}

func SaveProfile(p *models.Profile) error {
	return storage.WriteJSON(storage.ProfilePath(), p)
}
