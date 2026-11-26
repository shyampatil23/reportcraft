package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"performancereport/models"
	"performancereport/storage"
)

type gitlabCreds struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

func loadGitLabCreds() (*gitlabCreds, error) {
	var c gitlabCreds
	if err := storage.ReadJSON(storage.GitLabCredsPath(), &c); err != nil {
		return nil, err
	}
	if c.URL == "" {
		c.URL = "https://gitlab.com"
	}
	return &c, nil
}

// GetGitLabCommits fetches commits from GitLab for a given project and month.
func GetGitLabCommits(projectID, month string) ([]models.GitLabCommit, error) {
	creds, err := loadGitLabCreds()
	if err != nil || creds.Token == "" {
		return nil, fmt.Errorf("GitLab not configured")
	}

	t, err := time.Parse("2006-01", month)
	if err != nil {
		return nil, fmt.Errorf("invalid month: %s", month)
	}

	since := t.Format(time.RFC3339)
	until := t.AddDate(0, 1, 0).Format(time.RFC3339)

	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/commits?since=%s&until=%s&per_page=100",
		creds.URL,
		url.PathEscape(projectID),
		url.QueryEscape(since),
		url.QueryEscape(until),
	)

	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("PRIVATE-TOKEN", creds.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var raw []struct {
		ID            string `json:"id"`
		Title         string `json:"title"`
		Message       string `json:"message"`
		AuthorName    string `json:"author_name"`
		CommittedDate string `json:"committed_date"`
		WebURL        string `json:"web_url"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse error: %w — body: %s", err, string(body))
	}

	commits := make([]models.GitLabCommit, 0, len(raw))
	for _, c := range raw {
		commits = append(commits, models.GitLabCommit{
			ID:            c.ID,
			Title:         c.Title,
			Message:       c.Message,
			AuthorName:    c.AuthorName,
			CommittedDate: c.CommittedDate,
			WebURL:        c.WebURL,
			ProjectName:   projectID,
		})
	}
	return commits, nil
}

// SaveGitLabCreds stores GitLab credentials securely in the credentials folder.
func SaveGitLabCreds(gitlabURL, token string) error {
	creds := gitlabCreds{URL: gitlabURL, Token: token}
	return storage.WriteJSON(storage.GitLabCredsPath(), &creds)
}
