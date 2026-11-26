# Monthly Performance Report Generator

An AI-powered, local-first Monthly Performance Report Generator built with **Go** and **Vue 3**.

Runs locally on Windows 11 without any database or central server.

---

## 🚀 Key Features

### 1. Two Task Entry Modes
- **Daily Task Entry**:
  - Interactive calendar handling 28, 29 (leap years), 30, and 31 days.
  - Record daily contributions: Project, Title, Short Description, Detailed Description, Status, Tags, and structured report sections.
  - Actions: Add, Edit, Delete, Duplicate, and AI Writing Assistant.
- **Full Month Entry**:
  - Large text editor to paste an entire month's work notes, commit logs, or ticket exports.
  - **Offline Heading & Description Detection**: Automatically detects project headings (e.g., `EMACloudContentManager`, `ERA 3.0`), task titles, and body paragraphs without requiring internet.
  - **Structured Review Screen**: Edit task cards grouped by project, rename projects, split/merge descriptions, inspect ambiguity warnings, and confirm import into monthly tasks.

### 2. Professional Document Generation
- **Default MKCL ERA Template**:
  - **Page 1 (Cover Page)**: Purple title banner (`#4F46E5`), reporting month/year, projects worked on, employee name, employee code, appraiser name, and department.
  - **Page 2 (Table of Contents / Index)**: Automatically generated from project sections and task headings.
  - **Page 3+ (Detailed Task Sections)**: Grouped by project banner. Structured fields include *Task Title*, *Issue Identified*, *Activities Performed*, *Solution Implemented*, and *Outcome*.
  - **Final Page (Conclusion)**: Synthesizes accomplishments across all projects with editable text.
- **Dual Export Pipeline**:
  - Editable Microsoft Word document (`PerformanceReport<Month><Year>.docx`) generated locally with real WordprocessingML XML structures.
  - Print-ready PDF document (`PerformanceReport<Month><Year>.pdf`) with identical typography and styling.

### 3. Local-First Architecture
- Data persisted under `%LOCALAPPDATA%\PerformanceReportBuilder\data`:
  - `profile.json`
  - `tasks/YYYY-MM.json`
  - `full-month-drafts/YYYY-MM.json`
  - `reports/YYYY-MM/draft.json`
  - `templates/`
  - `backups/`
- Zero external database dependencies (No Postgres, Mongo, MySQL, or SQLite).
- Concurrency-safe atomic writes (write temp, atomic rename).
- Built-in one-click ZIP backup and restore.

### 4. Optional Integrations
- **AI Writing Assistant**: Expand short bullet points into formal technical documentation (compatible with OpenAI and Google Gemini APIs). Gracefully falls back when offline.
- **GitLab Integration**: Import commit messages and merge requests within a selected month.

---

## 🛠️ Tech Stack

- **Backend**: Go 1.26 (Gorilla Mux, local static file serving, XML DOCX generator)
- **Frontend**: Vue 3 (Composition API, `<script setup lang="ts">`), TypeScript, Vite, Pinia, Vue Router, Tailwind CSS, Lucide icons, Date-fns.

---

## 📦 How to Run

### Quick Start (Standalone Executable)
Double-click or run:
```powershell
.\PerformanceReport.exe
```
This starts the local Go web server on `http://127.0.0.1:8765`, serves the embedded Vue frontend, and automatically opens the application in your default web browser.

### Development Mode
To develop frontend and backend with hot-reload:
```powershell
# In terminal 1 (Go backend API):
go run main.go -port 8765

# In terminal 2 (Vue frontend with Vite dev server):
cd frontend
npm run dev
```
Open `http://localhost:5173` in your browser. Requests to `/api` are automatically proxied to the Go server on port 8765.

---

## 🧪 Running Tests

To verify local persistence, leap years, offline parsing, and DOCX/PDF generation:
```powershell
$env:PERFORMANCE_REPORT_DATA_DIR="d:\GoWorkspace\src\PerformanceReport\testdata"
go test -v ./services
```

---

## 📁 Repository Structure

```
PerformanceReport/
├── PerformanceReport.exe     # Standalone compiled Windows executable
├── main.go                   # Go entry point (embeds frontend/dist)
├── server/                   # HTTP server & routing
├── handlers/                 # REST API handlers
├── services/                 # Task, Report, Parser, DOCX/PDF, Backup services
├── models/                   # Typed Go data structures
├── storage/                  # Concurrency-safe JSON file persistence
├── build.ps1                 # Automated full build script
├── frontend/                 # Vue 3 TypeScript project
│   ├── src/
│   │   ├── views/            # Dashboard, DailyTasks, FullMonthEntry, etc.
│   │   ├── stores/           # Pinia stores (profile, tasks, reports, ui)
│   │   ├── api/              # Axios API clients
│   │   └── types/            # TypeScript data models
│   └── dist/                 # Production compiled SPA bundle
└── testdata/                 # Test artifacts & generated sample reports
```
