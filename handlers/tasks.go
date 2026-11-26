package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"performancereport/models"
	"performancereport/services"
)

func GetTasks(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	tasks, err := services.GetTasks(month)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, tasks)
}

func CreateTask(w http.ResponseWriter, r *http.Request) {
	var task models.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		jsonError(w, "invalid body: "+err.Error(), 400)
		return
	}
	if task.Date == "" {
		jsonError(w, "date is required", 400)
		return
	}
	month := task.Date[:7]
	if err := services.SaveTask(month, &task); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, task)
}

func UpdateTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var task models.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		jsonError(w, "invalid body: "+err.Error(), 400)
		return
	}
	task.ID = id
	month := task.Date[:7]
	task.UpdatedAt = time.Now()
	if err := services.SaveTask(month, &task); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, task)
}

func DeleteTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	month := r.URL.Query().Get("month")
	if month == "" {
		jsonError(w, "month query param required", 400)
		return
	}
	if err := services.DeleteTask(month, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

func BulkImportTasks(w http.ResponseWriter, r *http.Request) {
	var req models.BulkTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body: "+err.Error(), 400)
		return
	}
	if err := services.BulkImportTasks(req.Month, req.Tasks); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	tasks, _ := services.GetTasks(req.Month)
	jsonOK(w, tasks)
}

func GetTaskStats(w http.ResponseWriter, r *http.Request) {
	month := r.URL.Query().Get("month")
	if month == "" {
		now := time.Now()
		month = now.Format("2006-01")
	}
	stats, err := services.GetTaskStats(month)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, stats)
}

func GetAllTasks(w http.ResponseWriter, r *http.Request) {
	// Return tasks from month in query or current month
	month := r.URL.Query().Get("month")
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	tasks, err := services.GetTasks(month)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, tasks)
}

func ParseMonthlyText(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	body.Text = strings.TrimSpace(body.Text)
	if body.Text == "" {
		jsonError(w, "text is required", 400)
		return
	}
	result := services.ParseMonthlyText(body.Text)
	jsonOK(w, result)
}

func GetFullMonthDraft(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	month := vars["year"] + "-" + vars["month"]
	draft, err := services.GetFullMonthDraft(month)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, draft)
}

func SaveFullMonthDraft(w http.ResponseWriter, r *http.Request) {
	var draft models.FullMonthDraft
	if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
		jsonError(w, "invalid body: "+err.Error(), 400)
		return
	}
	if err := services.SaveFullMonthDraft(&draft); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, draft)
}
