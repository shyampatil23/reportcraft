package handlers

import (
	"encoding/json"
	"net/http"

	"performancereport/models"
	"performancereport/services"
)

func GetProfile(w http.ResponseWriter, r *http.Request) {
	p, err := services.GetProfile()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, p)
}

func UpdateProfile(w http.ResponseWriter, r *http.Request) {
	var p models.Profile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		jsonError(w, "invalid body: "+err.Error(), 400)
		return
	}
	if err := services.SaveProfile(&p); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, p)
}
