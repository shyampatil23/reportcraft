package services

import (
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"performancereport/models"
	"performancereport/storage"
)

func GetTasks(month string) ([]models.Task, error) {
	var tasks []models.Task
	if err := storage.ReadJSON(storage.TasksPath(month), &tasks); err != nil {
		return nil, err
	}
	if tasks == nil {
		tasks = []models.Task{}
	}
	return tasks, nil
}

func SaveTask(month string, task *models.Task) error {
	tasks, err := GetTasks(month)
	if err != nil {
		return err
	}
	if task.ID == "" {
		task.ID = uuid.New().String()
	}
	now := time.Now()
	task.UpdatedAt = now
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}

	found := false
	for i, t := range tasks {
		if t.ID == task.ID {
			tasks[i] = *task
			found = true
			break
		}
	}
	if !found {
		tasks = append(tasks, *task)
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Date < tasks[j].Date
	})
	return storage.WriteJSON(storage.TasksPath(month), tasks)
}

func DeleteTask(month, id string) error {
	tasks, err := GetTasks(month)
	if err != nil {
		return err
	}
	newTasks := make([]models.Task, 0, len(tasks))
	for _, t := range tasks {
		if t.ID != id {
			newTasks = append(newTasks, t)
		}
	}
	return storage.WriteJSON(storage.TasksPath(month), newTasks)
}

func BulkImportTasks(month string, newTasks []models.Task) error {
	existing, err := GetTasks(month)
	if err != nil {
		return err
	}
	existingIDs := make(map[string]bool)
	for _, t := range existing {
		existingIDs[t.ID] = true
	}
	now := time.Now()
	for i := range newTasks {
		if newTasks[i].ID == "" {
			newTasks[i].ID = uuid.New().String()
		}
		if newTasks[i].CreatedAt.IsZero() {
			newTasks[i].CreatedAt = now
		}
		newTasks[i].UpdatedAt = now
		if !existingIDs[newTasks[i].ID] {
			existing = append(existing, newTasks[i])
		}
	}
	sort.Slice(existing, func(i, j int) bool {
		return existing[i].Date < existing[j].Date
	})
	return storage.WriteJSON(storage.TasksPath(month), existing)
}

func GetTaskStats(month string) (*models.TaskStats, error) {
	tasks, err := GetTasks(month)
	if err != nil {
		return nil, err
	}
	stats := &models.TaskStats{
		TotalTasks:  len(tasks),
		Projects:    []string{},
		RecentTasks: []models.Task{},
	}
	days := make(map[string]bool)
	projectSet := make(map[string]bool)
	for _, t := range tasks {
		days[t.Date] = true
		projectSet[t.Project] = true
		if t.Status == "completed" {
			stats.CompletedTasks++
		}
	}
	stats.DaysWithEntries = len(days)
	for p := range projectSet {
		if p != "" {
			stats.Projects = append(stats.Projects, p)
		}
	}

	recent := tasks
	if len(recent) > 5 {
		recent = recent[len(recent)-5:]
	}
	for i := len(recent) - 1; i >= 0; i-- {
		stats.RecentTasks = append(stats.RecentTasks, recent[i])
	}

	// Latest report
	reports, _ := ListReports()
	if len(reports) > 0 {
		stats.LatestReport = reports[0].Month
	}

	return stats, nil
}

// MonthFromDate extracts YYYY-MM from YYYY-MM-DD
func MonthFromDate(date string) (string, error) {
	if len(date) < 7 {
		return "", fmt.Errorf("invalid date: %s", date)
	}
	return date[:7], nil
}
