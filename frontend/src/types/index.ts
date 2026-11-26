// Core data models matching Go structs

export interface Profile {
  employeeName: string
  employeeCode: string
  appraiserName: string
  department: string
  team: string
  projects: string[]
  updatedAt?: string
}

export interface Task {
  id: string
  date: string
  project: string
  title: string
  shortDescription: string
  detailedDescription: string
  issueIdentified: string
  activitiesPerformed: string
  solutionImplemented: string
  outcome: string
  status: 'pending' | 'in-progress' | 'completed'
  tags: string[]
  source: 'daily' | 'fullmonth'
  originalTaskIds?: string[]
  createdAt: string
  updatedAt: string
}

export interface ParsedTask {
  id: string
  project: string
  title: string
  shortDescription: string
  detailedDescription: string
  issueIdentified: string
  activitiesPerformed: string
  solutionImplemented: string
  outcome: string
  isAmbiguous: boolean
  ambiguityReason?: string
}

export interface ParseResult {
  tasks: ParsedTask[]
  projects: string[]
}

export interface FullMonthDraft {
  month: string
  rawText: string
  parsedTasks?: Task[]
  savedAt?: string
}

export interface CustomSubheading {
  heading: string
  content: string
}

export interface ReportTask {
  taskId: string
  title: string
  issueIdentified: string
  activitiesPerformed: string
  solutionImplemented: string
  outcome: string
  challengesFaced?: string
  customSections?: CustomSubheading[]
  order: number
}

export interface ReportSection {
  project: string
  tasks: ReportTask[]
  order: number
}

export interface CoverPage {
  title: string
  month: string
  year: string
  monthYear: string
  employeeName: string
  employeeCode: string
  appraiserName: string
  department: string
  projects: string[]
}

export interface Report {
  id: string
  month: string
  templateId?: string
  formattingMode?: 'match_template' | 'custom'
  schemaVersion?: number
  coverPage: CoverPage
  sections: ReportSection[]
  conclusion: string
  version: number
  status: 'draft' | 'final'
  generatedAt: string
  updatedAt: string
}

export interface TaskStats {
  totalTasks: number
  daysWithEntries: number
  completedTasks: number
  projects: string[]
  latestReport: string
  recentTasks: Task[]
}

export interface TextStyle {
  fontFamily: string
  fontSizePt: number
  bold: boolean
  italic?: boolean
  color: string
  align?: 'left' | 'center' | 'right' | 'justify'
}

export interface TemplatePageConfig {
  size: string
  orientation: 'portrait' | 'landscape'
  widthMM: number
  heightMM: number
  marginTopMM: number
  marginBottomMM: number
  marginLeftMM: number
  marginRightMM: number
  lineSpacing: number
  paragraphSpacingPt: number
  bulletIndentMM: number
  showPageNumbers: boolean
  pageNumberStyle: string
  footerText: string
}

export interface TemplateTypography {
  bannerTitle: TextStyle
  coverProjects: TextStyle
  coverEmployee: TextStyle
  tocTitle: TextStyle
  tocProject: TextStyle
  tocTask: TextStyle
  projectName: TextStyle
  taskTitle: TextStyle
  subheading: TextStyle
  taskDescription: TextStyle
  conclusionTitle: TextStyle
  conclusionBody: TextStyle
}

export interface TemplateAssets {
  coverHeroImage?: string
  logoImage?: string
  headerImage?: string
  footerImage?: string
  backgroundImage?: string
  extractedFiles?: string[]
}

export interface TemplateFieldMapping {
  id: string
  fieldType: string
  label: string
  pageNumber: number
  x: number
  y: number
  width: number
  height: number
  fontFamily: string
  fontSizePt: number
  bold: boolean
  color: string
  maskExisting?: boolean
}

export interface TemplateAnalysisResult {
  pageCount: number
  pageWidthPt: number
  pageHeightPt: number
  orientation: string
  marginTopPt: number
  marginBottomPt: number
  marginLeftPt: number
  marginRightPt: number
  fonts: string[]
  fontSizesPt: number[]
  colors: string[]
  imagesCount: number
  hasTOC: boolean
  detectedHeadings: string[]
  notes: string[]
}

export interface Template {
  id: string
  name: string
  description: string
  isDefault: boolean
  sourceType: 'era-builtin' | 'docx' | 'pdf' | 'custom'
  sourceFile?: string
  formattingMode: 'match_template' | 'custom'
  approved: boolean
  schemaVersion?: number
  primaryColor: string
  accentColor: string
  logoPath?: string
  sections: string[]
  pageConfig: TemplatePageConfig
  typography: TemplateTypography
  assets: TemplateAssets
  fieldMappings?: TemplateFieldMapping[]
  detectedInfo?: TemplateAnalysisResult
  createdAt: string
  updatedAt: string
}

export interface Settings {
  aiEnabled: boolean
  aiProvider: string
  aiModel: string
  gitlabEnabled: boolean
  gitlabUrl: string
  autoBackup: boolean
  backupFrequency: string
  dataPath: string
  updatedAt?: string
}

export interface AIImproveRequest {
  text: string
  mode: 'improve' | 'expand' | 'activities' | 'issue' | 'solution' | 'outcome' | 'bullets' | 'conclusion' | 'title' | 'challenges' | 'organize' | 'suggest_mapping'
  detail: 'concise' | 'standard' | 'detailed'
  context?: string
}

export interface AIImproveResponse {
  original: string
  improved: string
  mode: string
}

export interface BackupInfo {
  filename: string
  createdAt: string
  sizeMB: number
}

export interface GitLabCommit {
  id: string
  title: string
  message: string
  authorName: string
  committedDate: string
  webUrl: string
  projectName: string
}

export interface ApiResponse<T> {
  data?: T
  error?: string
}
