import axios from 'axios'
import type {
  Profile, Task, FullMonthDraft, ParseResult, Report,
  TaskStats, Template, Settings, AIImproveRequest, AIImproveResponse,
  BackupInfo, GitLabCommit
} from '@/types'

const api = axios.create({
  baseURL: '/api',
  timeout: 15000,
  headers: { 'Content-Type': 'application/json' },
})

function unwrap<T>(response: any): T {
  if (response.data?.data !== undefined) return response.data.data as T
  return response.data as T
}

// Profile
export const profileApi = {
  get: () => api.get<any>('/profile').then(unwrap<Profile>),
  update: (p: Profile) => api.put<any>('/profile', p).then(unwrap<Profile>),
}

// Tasks
export const tasksApi = {
  getMonth: (year: string, month: string) =>
    api.get<any>(`/tasks/${year}/${month}`).then(unwrap<Task[]>),
  getAll: (month: string) =>
    api.get<any>('/tasks', { params: { month } }).then(unwrap<Task[]>),
  create: (task: Partial<Task>) => api.post<any>('/tasks', task).then(unwrap<Task>),
  update: (id: string, task: Partial<Task>) =>
    api.put<any>(`/tasks/${id}`, task).then(unwrap<Task>),
  delete: (id: string, month: string) =>
    api.delete(`/tasks/${id}`, { params: { month } }),
  bulkImport: (month: string, tasks: Task[]) =>
    api.post<any>('/tasks/bulk', { month, tasks }).then(unwrap<Task[]>),
  getStats: (month: string) =>
    api.get<any>('/stats', { params: { month } }).then(unwrap<TaskStats>),
}

// Full Month Entry
export const fullMonthApi = {
  get: (year: string, month: string) =>
    api.get<any>(`/fullmonth/${year}/${month}`).then(unwrap<FullMonthDraft>),
  save: (year: string, month: string, draft: FullMonthDraft) =>
    api.put<any>(`/fullmonth/${year}/${month}`, draft).then(unwrap<FullMonthDraft>),
  parse: (text: string) =>
    api.post<any>('/fullmonth/parse', { text }).then(unwrap<ParseResult>),
}

// Reports
export interface MonthlyReportStatus {
  month: string
  status: 'IN_PROGRESS' | 'GENERATED_DRAFT' | 'EDITED_DRAFT' | 'COMPLETED'
  generatedAt?: string | null
  editedAt?: string | null
  completedAt?: string | null
  lastReminderAt?: string | null
  remindLaterUntil?: string | null
  pdfPath?: string | null
  docxPath?: string | null
}

export interface PendingMonthReminder {
  month: string
  monthName: string
  monthLabel: string
  status: string
  taskCount: number
}

export interface ReminderEvaluation {
  currentDate: string
  currentMonth: string
  currentMonthName: string
  isLastDayOfMonth: boolean
  currentMonthStatus: MonthlyReportStatus
  pendingReminder?: PendingMonthReminder | null
  draftNotice?: PendingMonthReminder | null
  tasksChangedWarning: boolean
}

export interface MissingTaskSummary {
  month: string
  monthLabel: string
  totalCalendarDays: number
  daysWithEntries: number
  totalTasks: number
  datesWithoutEntries: string[]
}

export const reportsApi = {
  list: () => api.get<any>('/reports').then(unwrap<Report[]>),
  history: () => api.get<any>('/reports/history').then(unwrap<any[]>),
  reminders: (date?: string) =>
    api.get<any>('/reports/reminders', { params: date ? { date } : undefined }).then(unwrap<ReminderEvaluation>),
  get: (year: string, month: string) =>
    api.get<any>(`/reports/${year}/${month}`).then(unwrap<Report>),
  status: (year: string, month: string) =>
    api.get<any>(`/reports/${year}/${month}/status`).then(unwrap<{ statusInfo: MonthlyReportStatus; tasksChangedWarning: boolean }>),
  remindLater: (year: string, month: string) =>
    api.post<any>(`/reports/${year}/${month}/remind-later`).then(unwrap<MonthlyReportStatus>),
  complete: (year: string, month: string) =>
    api.post<any>(`/reports/${year}/${month}/complete`).then(unwrap<MonthlyReportStatus>),
  missingSummary: (year: string, month: string) =>
    api.get<any>(`/reports/${year}/${month}/missing-summary`).then(unwrap<MissingTaskSummary>),
  generate: (month: string, selectedTaskIds?: string[], templateId?: string, formattingMode?: string, forceOverwriteEdits?: boolean) =>
    api.post<any>('/reports/generate', { month, selectedTaskIds: selectedTaskIds || [], templateId, formattingMode, forceOverwriteEdits }).then(unwrap<Report>),
  update: (year: string, month: string, report: Report) =>
    api.put<any>(`/reports/${year}/${month}`, report).then(unwrap<Report>),
  exportPdfUrl: (year: string, month: string, dir?: string) =>
    `/api/reports/${year}/${month}/export/pdf${dir ? '?dir=' + encodeURIComponent(dir) : ''}`,
  exportDocxUrl: (year: string, month: string) => `/api/reports/${year}/${month}/export/docx`,
  previewUrl: (year: string, month: string) => `/api/reports/${year}/${month}/preview`,
}

// Document Editor
export interface EditorPageSettings {
  pageSize: 'Letter' | 'A4' | string
  marginTopMM: number
  marginRightMM: number
  marginBottomMM: number
  marginLeftMM: number
  headerText: string
  footerText: string
}

export interface EditedDocument {
  month: string
  htmlContent: string
  pageSettings: EditorPageSettings
  isDraft: boolean
  updatedAt: string
  sourceMode: string
}

export interface EditorStatus {
  hasDraft: boolean
  hasSavedEdits: boolean
  updatedAt?: string
  draftUpdatedAt?: string
  sourceMode?: string
}

export const editorApi = {
  get: (year: string, month: string) =>
    api.get<any>(`/reports/${year}/${month}/editor`).then(unwrap<{ document: EditedDocument; status: EditorStatus }>),
  status: (year: string, month: string) =>
    api.get<any>(`/reports/${year}/${month}/editor/status`).then(unwrap<EditorStatus>),
  saveDraft: (year: string, month: string, doc: EditedDocument) =>
    api.post<any>(`/reports/${year}/${month}/editor/draft`, doc).then(unwrap<EditorStatus>),
  saveChanges: (year: string, month: string, doc: EditedDocument) =>
    api.post<any>(`/reports/${year}/${month}/editor/save`, doc, { timeout: 60000 }).then(unwrap<EditorStatus>),
  discard: (year: string, month: string) =>
    api.post<any>(`/reports/${year}/${month}/editor/discard`).then(unwrap<{ document: EditedDocument; status: EditorStatus }>),
  openInWord: (year: string, month: string) =>
    api.post<any>(`/reports/${year}/${month}/editor/open-word`).then(unwrap<{ status: string; path: string }>),
  syncFromWord: (year: string, month: string) =>
    api.post<any>(`/reports/${year}/${month}/editor/sync-word`, {}, { timeout: 60000 }).then(unwrap<EditorStatus>),
  clear: (year: string, month: string) =>
    api.post<any>(`/reports/${year}/${month}/editor/clear`).then(unwrap<EditorStatus>),
}

// Templates
export const templatesApi = {
  list: () => api.get<any>('/templates').then(unwrap<Template[]>),
  importFile: (file: File, name: string, primaryColor: string, accentColor: string) => {
    const formData = new FormData()
    formData.append('file', file)
    formData.append('name', name)
    formData.append('primaryColor', primaryColor)
    formData.append('accentColor', accentColor)
    return api.post<any>('/templates/import', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 30000,
    }).then(unwrap<Template>)
  },
  update: (id: string, tmpl: Template) =>
    api.put<any>(`/templates/${id}`, tmpl).then(unwrap<Template>),
  duplicate: (id: string) =>
    api.post<any>(`/templates/${id}/duplicate`).then(unwrap<Template>),
  remove: (id: string) =>
    api.delete<any>(`/templates/${id}`),
  setDefault: (id: string) =>
    api.post<any>(`/templates/${id}/default`).then(unwrap<Template>),
  sourceUrl: (id: string) => `/api/templates/${id}/source`,
  assetUrl: (id: string, filename: string) => `/api/templates/${id}/assets/${encodeURIComponent(filename)}`,
  previewUrl: (id: string) => `/api/templates/${id}/preview`,
}

// Settings
export const settingsApi = {
  get: () => api.get<any>('/settings').then(unwrap<Settings>),
  update: (s: Settings) => api.put<any>('/settings', s).then(unwrap<Settings>),
}

// Backup
export const backupApi = {
  create: () => api.post<any>('/backup').then(unwrap<BackupInfo>),
  list: () => api.get<any>('/backup').then(unwrap<BackupInfo[]>),
  dataFolder: () => api.get<any>('/data-folder').then(unwrap<{ path: string }>),
}

// AI
export const aiApi = {
  getCreds: () =>
    api.get<any>('/ai/creds').then(unwrap<{ provider: string; apiKey: string; model: string; baseUrl: string }>),
  improve: (req: AIImproveRequest) =>
    api.post<any>('/ai/improve', req).then(unwrap<AIImproveResponse>),
  test: () =>
    api.post<any>('/ai/test').then(unwrap<{ status: string; provider: string; model: string; latencyMs: number }>),
  saveCreds: (provider: string, apiKey: string, model: string, baseUrl: string) =>
    api.post('/ai/creds', { provider, apiKey, model, baseUrl }),
}

// GitLab
export const gitlabApi = {
  getCommits: (project: string, month: string) =>
    api.get<any>('/gitlab/commits', { params: { project, month } }).then(unwrap<GitLabCommit[]>),
}

export default api
