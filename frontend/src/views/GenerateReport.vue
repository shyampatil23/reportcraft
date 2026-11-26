<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { format } from 'date-fns'
import {
  Check, ChevronRight, ChevronLeft, Download, FileText,
  Sparkles, Settings, Eye, ArrowRight, Plus, Trash2, Copy,
  ZoomIn, ZoomOut, ArrowUp, ArrowDown, Pencil
} from 'lucide-vue-next'
import DocumentEditor from '@/components/DocumentEditor.vue'
import { useReportsStore } from '@/stores/reports'
import { useTasksStore } from '@/stores/tasks'
import { useProfileStore } from '@/stores/profile'
import { useUIStore } from '@/stores/ui'
import { reportsApi, templatesApi, editorApi, type EditorStatus, type MonthlyReportStatus, type MissingTaskSummary } from '@/api'
import type { Task, Template, Report, ReportTask, ReportSection, CoverPage } from '@/types'

// ─── Stores ────────────────────────────────────────────────────────────────────
const route = useRoute()
const reportsStore = useReportsStore()
const tasksStore = useTasksStore()
const profileStore = useProfileStore()
const uiStore = useUIStore()

// ─── Wizard State ──────────────────────────────────────────────────────────────
const currentStep = ref(1)
const loading = ref(false)

// Step 1 state
const now = new Date()
const selectedMonth = ref(now.getMonth() + 1) // 1-12
const selectedYear = ref(now.getFullYear())
const templates = ref<Template[]>([])
const selectedTemplateId = ref<string>('')
const formattingMode = ref<'match_template' | 'custom'>('match_template')
const previewZoom = ref(1.0)
const downloadDir = ref('')
const step5Mode = ref<'preview' | 'edit'>('preview')
const editorStatus = ref<EditorStatus>({ hasDraft: false, hasSavedEdits: false })
const ambiguousWarning = ref('')
const monthStatus = ref<MonthlyReportStatus | null>(null)
const tasksChangedWarning = ref(false)
const missingSummary = ref<MissingTaskSummary | null>(null)
const showMissingDates = ref(false)
const completingReport = ref(false)
const showRegenChoiceModal = ref(false)

async function loadMonthMeta() {
  try {
    const [stRes, sumRes] = await Promise.all([
      reportsApi.status(yearStr.value, monthPadded.value),
      reportsApi.missingSummary(yearStr.value, monthPadded.value),
    ])
    monthStatus.value = stRes.statusInfo
    tasksChangedWarning.value = stRes.tasksChangedWarning
    missingSummary.value = sumRes
  } catch {
    // ignore
  }
}

async function markAsCompleted() {
  if (editorStatus.value.hasDraft) {
    uiStore.notify('Please save or discard your unsaved draft in Edit Document before marking as completed.', 'error')
    return
  }
  completingReport.value = true
  try {
    monthStatus.value = await reportsApi.complete(yearStr.value, monthPadded.value)
    uiStore.notify('Report marked as COMPLETED and archived in Report History!', 'success')
  } catch (e: any) {
    uiStore.notify(`Failed to complete report: ${e?.message || 'Unknown error'}`, 'error')
  } finally {
    completingReport.value = false
  }
}

// Step 2 state
const selectedTaskIds = ref<string[]>([])
const monthTasks = ref<Task[]>([])
const tasksLoading = ref(false)

// Step 3 state — ordered list of tasks (cloned from monthTasks for reordering)
interface OrderedTask {
  taskId: string
  project: string
  title: string
  date: string
  order: number
}
const orderedTasks = ref<OrderedTask[]>([])

// Step 4 state — editable cover + tasks content
const coverForm = ref<CoverPage>({
  title: 'Monthly Performance Report',
  month: '',
  year: '',
  monthYear: '',
  employeeName: '',
  employeeCode: '',
  appraiserName: '',
  department: '',
  projects: [],
})
// Step 4 state — dynamic editable sections
interface EditableTask extends ReportTask {
  expanded: boolean
}
interface EditableProjectSection {
  project: string
  tasks: EditableTask[]
}
const editableSections = ref<EditableProjectSection[]>([])
const editableTasks = ref<Array<ReportTask & { expanded: boolean }>>([])
const conclusionText = ref('')

// Step 5 & 6 state
const reportData = ref<Report | null>(null)
const previewUrl = ref('')
const previewKey = ref(0)

// ─── Constants ─────────────────────────────────────────────────────────────────
const MONTHS = [
  { value: 1, label: 'January' },
  { value: 2, label: 'February' },
  { value: 3, label: 'March' },
  { value: 4, label: 'April' },
  { value: 5, label: 'May' },
  { value: 6, label: 'June' },
  { value: 7, label: 'July' },
  { value: 8, label: 'August' },
  { value: 9, label: 'September' },
  { value: 10, label: 'October' },
  { value: 11, label: 'November' },
  { value: 12, label: 'December' },
]

const YEARS = [2024, 2025, 2026, 2027]

const STEPS = [
  { number: 1, label: 'Period & Template', icon: Settings },
  { number: 2, label: 'Select Tasks', icon: FileText },
  { number: 3, label: 'Organize', icon: ArrowRight },
  { number: 4, label: 'Edit Content', icon: Sparkles },
  { number: 5, label: 'Preview', icon: Eye },
  { number: 6, label: 'Download', icon: Download },
]

// ─── Computed ──────────────────────────────────────────────────────────────────
const monthPadded = computed(() =>
  String(selectedMonth.value).padStart(2, '0')
)

const yearStr = computed(() => String(selectedYear.value))

const monthLabel = computed(() =>
  MONTHS.find(m => m.value === selectedMonth.value)?.label ?? ''
)

const monthKey = computed(() => `${yearStr.value}-${monthPadded.value}`)

/** Tasks for selected month that are in selectedTaskIds */
const selectedTasks = computed(() =>
  monthTasks.value.filter(t => selectedTaskIds.value.includes(t.id))
)

/** Tasks grouped by project (from monthTasks) */
const tasksByProject = computed(() => {
  const map: Record<string, Task[]> = {}
  for (const t of monthTasks.value) {
    if (!map[t.project]) map[t.project] = []
    map[t.project].push(t)
  }
  return map
})

/** Ordered tasks grouped by project */
const orderedByProject = computed(() => {
  const map: Record<string, OrderedTask[]> = {}
  for (const t of orderedTasks.value) {
    if (!map[t.project]) map[t.project] = []
    map[t.project].push(t)
  }
  // Sort each project's tasks by order
  for (const p in map) {
    map[p].sort((a, b) => a.order - b.order)
  }
  return map
})

/** All projects in the ordered list */
const orderedProjects = computed(() => Object.keys(orderedByProject.value).sort())

/** All projects with selected tasks (Step 2 view) */
const projectsWithSelected = computed(() => {
  const set = new Set(selectedTasks.value.map(t => t.project))
  return Array.from(set).sort()
})

/** Are all currently loaded tasks selected? */
const allSelected = computed(() =>
  monthTasks.value.length > 0 &&
  monthTasks.value.every(t => selectedTaskIds.value.includes(t.id))
)

/** Editable tasks grouped by project */
const editableByProject = computed(() => {
  const map: Record<string, Array<ReportTask & { expanded: boolean }>> = {}
  for (const t of editableTasks.value) {
    const proj = orderedTasks.value.find(ot => ot.taskId === t.taskId)?.project ?? 'General'
    if (!map[proj]) map[proj] = []
    map[proj].push(t)
  }
  return map
})

const editableProjects = computed(() => Object.keys(editableByProject.value).sort())

/** PDF / docx filenames matching PerformanceReport<MonthName><Year> */
const pdfFilename = computed(() =>
  `PerformanceReport${monthLabel.value}${yearStr.value}.pdf`
)
const docxFilename = computed(() =>
  `PerformanceReport${monthLabel.value}${yearStr.value}.docx`
)

const pdfUrl = computed(() => reportsApi.exportPdfUrl(yearStr.value, monthPadded.value, downloadDir.value.trim() || undefined))
const docxUrl = computed(() => reportsApi.exportDocxUrl(yearStr.value, monthPadded.value))

// ─── Lifecycle ─────────────────────────────────────────────────────────────────
onMounted(async () => {
  // Load profile for cover page prefill
  if (!profileStore.profile) await profileStore.fetchProfile()

  // Load templates
  try {
    templates.value = await templatesApi.list()
    const def = templates.value.find(t => t.isDefault)
    if (def) selectedTemplateId.value = def.id
    else if (templates.value.length > 0) selectedTemplateId.value = templates.value[0].id
  } catch {
    templates.value = []
  }

  // Support direct jump from Monthly Tasks / Full Month Entry to Step 5 Preview
  const qMonth = typeof route.query.month === 'string' ? route.query.month : ''
  const qStep = typeof route.query.step === 'string' ? Number(route.query.step) : 0
  if (typeof route.query.ambiguous === 'string' && route.query.ambiguous.trim()) {
    ambiguousWarning.value = route.query.ambiguous.trim()
  }
  if (qMonth && /^\d{4}-\d{2}$/.test(qMonth)) {
    const [yr, mon] = qMonth.split('-')
    selectedYear.value = Number(yr)
    selectedMonth.value = Number(mon)
    try {
      const existing = await reportsApi.get(yr, mon)
      if (existing) {
        reportData.value = existing
        coverForm.value = { ...existing.coverPage }
        editableSections.value = (existing.sections || []).map(sec => ({
          project: sec.project,
          tasks: (sec.tasks || []).map(t => ({ ...t, expanded: false })),
        }))
        conclusionText.value = existing.conclusion || ''
        if (existing.templateId) selectedTemplateId.value = existing.templateId
        if (existing.formattingMode) formattingMode.value = existing.formattingMode
        previewUrl.value = reportsApi.previewUrl(yr, mon) + '?t=' + Date.now()
        previewKey.value++
        if (qStep === 5) currentStep.value = 5
      }
    } catch {
      // ignore if not generated yet
    }
  }
  await loadMonthMeta()
})

// ─── Watch ─────────────────────────────────────────────────────────────────────
watch([selectedMonth, selectedYear], () => {
  // Clear task selections when period changes
  selectedTaskIds.value = []
  monthTasks.value = []
  loadMonthMeta()
})

// ─── Step Navigation Helpers ───────────────────────────────────────────────────
function validateStep(step: number): boolean {
  switch (step) {
    case 1:
      if (!selectedTemplateId.value) {
        uiStore.notify('Please select a report template', 'error')
        return false
      }
      return true
    case 2:
      if (selectedTaskIds.value.length === 0) {
        uiStore.notify('Please select at least one task', 'error')
        return false
      }
      return true
    case 3:
      return true
    case 4:
      if (!coverForm.value.employeeName.trim()) {
        uiStore.notify('Employee name is required', 'error')
        return false
      }
      return true
    case 5:
      return true
    case 6:
      return true
    default:
      return true
  }
}

async function nextStep() {
  if (!validateStep(currentStep.value)) return

  if (currentStep.value === 1) {
    // Load tasks for selected month when moving to step 2
    await loadMonthTasks()
  } else if (currentStep.value === 2) {
    // Build ordered tasks from selected tasks
    buildOrderedTasks()
  } else if (currentStep.value === 3) {
    // Build editable content
    buildEditableContent()
  } else if (currentStep.value === 4) {
    // Generate report + load preview
    await generateAndPreview()
    if (!reportData.value) return // generation failed — stay on step 4
  }

  if (currentStep.value < 6) currentStep.value++
}

function prevStep() {
  if (currentStep.value > 1) currentStep.value--
}

function goToStep(n: number) {
  if (n < currentStep.value) currentStep.value = n
}

// ─── Step 2: Load Tasks ────────────────────────────────────────────────────────
async function loadMonthTasks() {
  tasksLoading.value = true
  try {
    const [year, mon] = [yearStr.value, monthPadded.value]
    const { tasksApi } = await import('@/api')
    monthTasks.value = await tasksApi.getMonth(year, mon) || []
  } catch (e) {
    uiStore.notify('Failed to load tasks for selected month', 'error')
    monthTasks.value = []
  } finally {
    tasksLoading.value = false
  }
}

function toggleSelectAll() {
  if (allSelected.value) {
    selectedTaskIds.value = []
  } else {
    selectedTaskIds.value = monthTasks.value.map(t => t.id)
  }
}

function toggleTask(id: string) {
  const idx = selectedTaskIds.value.indexOf(id)
  if (idx === -1) selectedTaskIds.value.push(id)
  else selectedTaskIds.value.splice(idx, 1)
}

function isTaskSelected(id: string) {
  return selectedTaskIds.value.includes(id)
}

function projectAllSelected(project: string) {
  const proj = tasksByProject.value[project] ?? []
  return proj.length > 0 && proj.every(t => selectedTaskIds.value.includes(t.id))
}

function toggleProjectAll(project: string) {
  const proj = tasksByProject.value[project] ?? []
  if (projectAllSelected(project)) {
    selectedTaskIds.value = selectedTaskIds.value.filter(
      id => !proj.some(t => t.id === id)
    )
  } else {
    for (const t of proj) {
      if (!selectedTaskIds.value.includes(t.id)) selectedTaskIds.value.push(t.id)
    }
  }
}

// ─── Step 3: Organize ─────────────────────────────────────────────────────────
function buildOrderedTasks() {
  let order = 0
  const list: OrderedTask[] = []

  // Keep existing order if already built, otherwise derive from selected
  const existingIds = new Set(orderedTasks.value.map(t => t.taskId))
  const newSelected = selectedTasks.value.filter(t => !existingIds.has(t.id))

  // Keep existing ordered tasks that are still selected
  const kept = orderedTasks.value.filter(t => selectedTaskIds.value.includes(t.taskId))

  // Append new tasks
  for (const t of kept) {
    list.push({ ...t, order: order++ })
  }
  for (const t of newSelected) {
    list.push({ taskId: t.id, project: t.project, title: t.title, date: t.date, order: order++ })
  }

  orderedTasks.value = list
}

function moveTaskUp(project: string, taskId: string) {
  const proj = orderedByProject.value[project]
  if (!proj) return
  const idx = proj.findIndex(t => t.taskId === taskId)
  if (idx <= 0) return
  const a = orderedTasks.value.find(t => t.taskId === proj[idx].taskId)!
  const b = orderedTasks.value.find(t => t.taskId === proj[idx - 1].taskId)!
  ;[a.order, b.order] = [b.order, a.order]
}

function moveTaskDown(project: string, taskId: string) {
  const proj = orderedByProject.value[project]
  if (!proj) return
  const idx = proj.findIndex(t => t.taskId === taskId)
  if (idx >= proj.length - 1) return
  const a = orderedTasks.value.find(t => t.taskId === proj[idx].taskId)!
  const b = orderedTasks.value.find(t => t.taskId === proj[idx + 1].taskId)!
  ;[a.order, b.order] = [b.order, a.order]
}

// ─── Step 4: Edit Content (Dynamic Projects & Tasks) ──────────────────────────
function buildEditableContent() {
  const p = profileStore.profile

  coverForm.value = {
    title: 'Monthly Performance Report',
    month: monthPadded.value,
    year: yearStr.value,
    monthYear: `${monthLabel.value} ${yearStr.value}`,
    employeeName: p?.employeeName ?? '',
    employeeCode: p?.employeeCode ?? '',
    appraiserName: p?.appraiserName ?? '',
    department: p?.department ?? '',
    projects: [...orderedProjects.value],
  }

  editableSections.value = orderedProjects.value.map(proj => {
    const tasks = (orderedByProject.value[proj] ?? []).map((ot, idx) => {
      const src = monthTasks.value.find(t => t.id === ot.taskId)
      return {
        taskId: ot.taskId,
        title: ot.title,
        issueIdentified: src?.issueIdentified ?? '',
        activitiesPerformed: src?.activitiesPerformed ?? src?.detailedDescription ?? '',
        solutionImplemented: src?.solutionImplemented ?? '',
        challengesFaced: '',
        customSections: [],
        outcome: src?.outcome ?? '',
        order: idx,
        expanded: false,
      }
    })
    return { project: proj, tasks }
  })

  editableTasks.value = editableSections.value.flatMap(s => s.tasks)

  const count = editableTasks.value.length
  const projectList = orderedProjects.value.join(', ')
  conclusionText.value =
    `During ${monthLabel.value} ${yearStr.value}, ${coverForm.value.employeeName || 'the employee'} ` +
    `successfully completed ${count} task${count !== 1 ? 's' : ''} across ${orderedProjects.value.length} ` +
    `project${orderedProjects.value.length !== 1 ? 's' : ''} (${projectList}). ` +
    `All deliverables were met with a focus on quality, collaboration, and continuous improvement.`
}

function syncCoverProjects() {
  coverForm.value.projects = editableSections.value
    .map(s => s.project.trim())
    .filter(p => p.length > 0)
}

function addProjectSection() {
  editableSections.value.push({
    project: `New Project ${editableSections.value.length + 1}`,
    tasks: [
      {
        taskId: 'custom-' + Date.now(),
        title: 'New Task',
        issueIdentified: '',
        activitiesPerformed: '',
        solutionImplemented: '',
        challengesFaced: '',
        customSections: [],
        outcome: '',
        order: 0,
        expanded: true,
      },
    ],
  })
  syncCoverProjects()
}

function deleteProjectSection(pIdx: number) {
  editableSections.value.splice(pIdx, 1)
  syncCoverProjects()
}

function moveProjectSection(pIdx: number, delta: number) {
  const target = pIdx + delta
  if (target < 0 || target >= editableSections.value.length) return
  const arr = editableSections.value
  ;[arr[pIdx], arr[target]] = [arr[target], arr[pIdx]]
  syncCoverProjects()
}

function addTaskToSection(pIdx: number) {
  const sec = editableSections.value[pIdx]
  if (!sec) return
  sec.tasks.push({
    taskId: 'custom-' + Date.now() + '-' + sec.tasks.length,
    title: 'New Task',
    issueIdentified: '',
    activitiesPerformed: '',
    solutionImplemented: '',
    challengesFaced: '',
    customSections: [],
    outcome: '',
    order: sec.tasks.length,
    expanded: true,
  })
}

function duplicateTaskInSection(pIdx: number, tIdx: number) {
  const sec = editableSections.value[pIdx]
  if (!sec) return
  const orig = sec.tasks[tIdx]
  const clone: EditableTask = JSON.parse(JSON.stringify(orig))
  clone.taskId = 'dup-' + Date.now()
  clone.title = orig.title + ' (Copy)'
  clone.expanded = true
  sec.tasks.splice(tIdx + 1, 0, clone)
}

function deleteTaskInSection(pIdx: number, tIdx: number) {
  editableSections.value[pIdx]?.tasks.splice(tIdx, 1)
}

function moveTaskInSection(pIdx: number, tIdx: number, delta: number) {
  const sec = editableSections.value[pIdx]
  if (!sec) return
  const target = tIdx + delta
  if (target < 0 || target >= sec.tasks.length) return
  ;[sec.tasks[tIdx], sec.tasks[target]] = [sec.tasks[target], sec.tasks[tIdx]]
}

function addCustomSubheading(task: EditableTask) {
  if (!task.customSections) task.customSections = []
  task.customSections.push({ heading: 'Additional Notes', content: '' })
}

function removeCustomSubheading(task: EditableTask, idx: number) {
  task.customSections?.splice(idx, 1)
}

function toggleExpandTask(task: EditableTask) {
  task.expanded = !task.expanded
}

// ─── Step 5: Generate & Preview ───────────────────────────────────────────────
async function generateAndPreview(forceFlag?: boolean) {
  loading.value = true
  try {
    let forceOverwrite = forceFlag === true
    if (forceFlag === undefined) {
      try {
        const [st, mSt] = await Promise.all([
          editorApi.status(yearStr.value, monthPadded.value),
          reportsApi.status(yearStr.value, monthPadded.value),
        ])
        editorStatus.value = st
        monthStatus.value = mSt.statusInfo
        tasksChangedWarning.value = mSt.tasksChangedWarning
        if (st.hasSavedEdits || st.hasDraft || mSt.tasksChangedWarning) {
          showRegenChoiceModal.value = true
          loading.value = false
          return
        }
      } catch {
        // ignore if status check fails
      }
    }

    syncCoverProjects()
    const sections: ReportSection[] = editableSections.value.map((sec, pIdx) => ({
      project: sec.project,
      order: pIdx,
      tasks: sec.tasks.map((et, tIdx) => ({
        taskId: et.taskId,
        title: et.title,
        issueIdentified: et.issueIdentified,
        activitiesPerformed: et.activitiesPerformed,
        solutionImplemented: et.solutionImplemented,
        challengesFaced: et.challengesFaced,
        customSections: et.customSections,
        outcome: et.outcome,
        order: tIdx,
      })),
    }))

    const generated = await reportsApi.generate(
      monthKey.value,
      selectedTaskIds.value,
      selectedTemplateId.value,
      formattingMode.value,
      forceOverwrite
    )

    const updated = await reportsApi.update(yearStr.value, monthPadded.value, {
      ...generated,
      templateId: selectedTemplateId.value,
      formattingMode: formattingMode.value,
      coverPage: coverForm.value,
      sections,
      conclusion: conclusionText.value,
    })

    reportData.value = updated
    previewUrl.value = reportsApi.previewUrl(yearStr.value, monthPadded.value) + '?t=' + Date.now()
    previewKey.value++
    await loadMonthMeta()
    uiStore.notify('Report generated successfully!', 'success')
  } catch (e: any) {
    uiStore.notify(`Failed to generate report: ${e?.message ?? 'Unknown error'}`, 'error')
    reportData.value = null
  } finally {
    loading.value = false
  }
}

async function regeneratePreview() {
  await generateAndPreview()
}

function onEditorSaved(st: EditorStatus) {
  editorStatus.value = st
  previewUrl.value = reportsApi.previewUrl(yearStr.value, monthPadded.value) + '?t=' + Date.now()
  previewKey.value++
  step5Mode.value = 'preview'
  loadMonthMeta()
}

// ─── Step 6: Download ─────────────────────────────────────────────────────────
function downloadPdf() {
  window.open(pdfUrl.value, '_blank')
}

function downloadDocx() {
  window.open(docxUrl.value, '_blank')
}

async function startNewVersion() {
  currentStep.value = 1
  selectedTaskIds.value = []
  orderedTasks.value = []
  editableTasks.value = []
  reportData.value = null
  previewUrl.value = ''
  uiStore.notify('Started a new version', 'info')
}
</script>

<template>
  <div class="min-h-screen bg-gray-50 dark:bg-gray-950 flex flex-col">
    <!-- ── Header ── -->
    <div class="bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-800 px-6 py-4">
      <div class="max-w-5xl mx-auto flex items-center gap-3">
        <div class="p-2 bg-primary-50 dark:bg-primary-900/30 rounded-lg">
          <FileText class="w-5 h-5 text-primary-600 dark:text-primary-400" />
        </div>
        <div>
          <h1 class="text-lg font-semibold text-gray-900 dark:text-white">Generate Report</h1>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ monthLabel }} {{ selectedYear }} • Step {{ currentStep }} of 6
          </p>
        </div>
      </div>
    </div>

    <!-- ── Step Indicator ── -->
    <div class="bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-800 px-6 py-4">
      <div class="max-w-5xl mx-auto">
        <div class="flex items-center justify-between">
          <template v-for="(step, idx) in STEPS" :key="step.number">
            <!-- Step circle + label -->
            <button
              class="flex flex-col items-center gap-1 group"
              :class="step.number < currentStep ? 'cursor-pointer' : 'cursor-default'"
              @click="goToStep(step.number)"
            >
              <div
                class="w-9 h-9 rounded-full flex items-center justify-center text-sm font-semibold transition-all duration-200"
                :class="{
                  'bg-primary-600 text-white shadow-md': step.number < currentStep,
                  'ring-2 ring-primary-600 ring-offset-2 bg-white dark:bg-gray-900 text-primary-600': step.number === currentStep,
                  'bg-gray-200 dark:bg-gray-700 text-gray-500 dark:text-gray-400': step.number > currentStep,
                }"
              >
                <Check v-if="step.number < currentStep" class="w-4 h-4" />
                <span v-else>{{ step.number }}</span>
              </div>
              <span
                class="text-xs font-medium hidden sm:block whitespace-nowrap"
                :class="{
                  'text-primary-600 dark:text-primary-400': step.number === currentStep,
                  'text-gray-700 dark:text-gray-300': step.number < currentStep,
                  'text-gray-400 dark:text-gray-500': step.number > currentStep,
                }"
              >{{ step.label }}</span>
            </button>

            <!-- Connector line (not after last) -->
            <div
              v-if="idx < STEPS.length - 1"
              class="flex-1 h-0.5 mx-2 rounded transition-colors duration-300"
              :class="step.number < currentStep ? 'bg-primary-500' : 'bg-gray-200 dark:bg-gray-700'"
            />
          </template>
        </div>
      </div>
    </div>

    <!-- ── Step Content ── -->
    <div class="flex-1 overflow-y-auto px-6 py-8">
      <div class="max-w-5xl mx-auto">

        <!-- ━━━ STEP 1: Period & Template ━━━ -->
        <div v-if="currentStep === 1" class="space-y-8">
          <div>
            <h2 class="text-xl font-semibold text-gray-900 dark:text-white mb-1">Select Period & Template</h2>
            <p class="text-sm text-gray-500 dark:text-gray-400">Choose the reporting month/year and a document template.</p>
          </div>

          <!-- Period selectors -->
          <div class="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
            <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider mb-4">Reporting Period</h3>
            <div class="flex flex-wrap gap-4">
              <div class="flex flex-col gap-1">
                <label class="text-xs text-gray-500 dark:text-gray-400 font-medium">Month</label>
                <select
                  v-model="selectedMonth"
                  class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
                >
                  <option v-for="m in MONTHS" :key="m.value" :value="m.value">{{ m.label }}</option>
                </select>
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-xs text-gray-500 dark:text-gray-400 font-medium">Year</label>
                <select
                  v-model="selectedYear"
                  class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
                >
                  <option v-for="y in YEARS" :key="y" :value="y">{{ y }}</option>
                </select>
              </div>
              <div class="flex items-end">
                <span class="text-sm font-medium text-gray-600 dark:text-gray-400 pb-2">
                  → {{ monthLabel }} {{ selectedYear }}
                </span>
              </div>
            </div>
          </div>

          <!-- Monthly Task Summary (Non-blocking Missing Dates) -->
          <div v-if="missingSummary" class="bg-white rounded-xl border border-gray-200 p-5 space-y-3">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <h3 class="text-sm font-semibold text-gray-800 uppercase tracking-wider">Monthly Task Summary — {{ missingSummary.monthLabel }}</h3>
              <div class="flex items-center gap-2">
                <button
                  v-if="missingSummary.datesWithoutEntries?.length"
                  class="px-3 py-1.5 rounded-lg border border-gray-300 text-xs font-medium hover:bg-gray-50"
                  @click="showMissingDates = !showMissingDates"
                >
                  {{ showMissingDates ? 'Hide Missing Dates' : `Review Missing Dates (${missingSummary.datesWithoutEntries.length})` }}
                </button>
                <button
                  class="px-3 py-1.5 rounded-lg bg-primary-600 text-white text-xs font-semibold hover:bg-primary-700"
                  @click="nextStep()"
                >
                  Continue to Generate Report
                </button>
              </div>
            </div>
            <div class="grid grid-cols-3 gap-3 text-xs">
              <div class="bg-gray-50 rounded-lg p-3">
                <span class="text-gray-500">Total Calendar Days</span>
                <div class="text-lg font-bold text-gray-900">{{ missingSummary.totalCalendarDays }}</div>
              </div>
              <div class="bg-blue-50 rounded-lg p-3">
                <span class="text-blue-700">Days With Entries</span>
                <div class="text-lg font-bold text-blue-900">{{ missingSummary.daysWithEntries }}</div>
              </div>
              <div class="bg-green-50 rounded-lg p-3">
                <span class="text-green-700">Total Tasks</span>
                <div class="text-lg font-bold text-green-900">{{ missingSummary.totalTasks }}</div>
              </div>
            </div>
            <div v-if="showMissingDates && missingSummary.datesWithoutEntries?.length" class="text-xs text-gray-600 bg-gray-50 rounded-lg p-3 max-h-28 overflow-y-auto">
              <span class="font-semibold">Dates Without Entries (weekends/holidays/leave are allowed):</span>
              <div class="flex flex-wrap gap-1.5 mt-1.5">
                <span v-for="d in missingSummary.datesWithoutEntries" :key="d" class="px-2 py-0.5 rounded bg-white border border-gray-200 font-mono text-[11px]">{{ d }}</span>
              </div>
            </div>
          </div>

          <!-- Template cards -->
          <div class="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
            <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider mb-4">Report Template</h3>

            <div v-if="templates.length === 0" class="text-center py-10 text-gray-400 dark:text-gray-500">
              <FileText class="w-10 h-10 mx-auto mb-2 opacity-40" />
              <p class="text-sm">No templates available</p>
            </div>

            <div v-else class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              <button
                v-for="tmpl in templates"
                :key="tmpl.id"
                class="relative text-left rounded-xl border-2 p-4 transition-all duration-200 hover:shadow-md"
                :class="selectedTemplateId === tmpl.id
                  ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/20'
                  : 'border-gray-200 dark:border-gray-700 hover:border-primary-300 dark:hover:border-primary-700'"
                @click="selectedTemplateId = tmpl.id"
              >
                <!-- Color swatch -->
                <div class="flex items-center gap-2 mb-3">
                  <div
                    class="w-5 h-5 rounded-full border-2 border-white dark:border-gray-900 shadow-sm"
                    :style="{ background: tmpl.primaryColor || '#6366f1' }"
                  />
                  <div
                    class="w-3 h-3 rounded-full"
                    :style="{ background: tmpl.accentColor || '#8b5cf6' }"
                  />
                </div>

                <p class="font-medium text-gray-900 dark:text-white text-sm">{{ tmpl.name }}</p>
                <p class="text-xs text-gray-500 dark:text-gray-400 mt-1 line-clamp-2">{{ tmpl.description }}</p>

                <div v-if="tmpl.isDefault" class="mt-2">
                  <span class="text-xs bg-primary-100 dark:bg-primary-900/40 text-primary-700 dark:text-primary-300 px-1.5 py-0.5 rounded">Default</span>
                </div>

                <!-- Selection tick -->
                <div
                  v-if="selectedTemplateId === tmpl.id"
                  class="absolute top-3 right-3 w-5 h-5 rounded-full bg-primary-600 flex items-center justify-center"
                >
                  <Check class="w-3 h-3 text-white" />
                </div>
              </button>
            </div>

            <!-- Formatting Mode Selector -->
            <div class="mt-6 pt-5 border-t border-gray-100 dark:border-gray-800">
              <label class="block text-xs font-semibold text-gray-600 dark:text-gray-400 uppercase tracking-wider mb-2">Formatting Mode</label>
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <button
                  type="button"
                  @click="formattingMode = 'match_template'"
                  class="text-left p-3.5 rounded-xl border-2 transition"
                  :class="formattingMode === 'match_template' ? 'border-primary-500 bg-primary-50/50 dark:bg-primary-900/20' : 'border-gray-200 dark:border-gray-700'"
                >
                  <div class="text-sm font-semibold text-gray-900 dark:text-white">Match Uploaded Template</div>
                  <div class="text-xs text-gray-500 mt-0.5">Preserves the selected template's exact fonts, sizes, and layout.</div>
                </button>
                <button
                  type="button"
                  @click="formattingMode = 'custom'"
                  class="text-left p-3.5 rounded-xl border-2 transition"
                  :class="formattingMode === 'custom' ? 'border-primary-500 bg-primary-50/50 dark:bg-primary-900/20' : 'border-gray-200 dark:border-gray-700'"
                >
                  <div class="text-sm font-semibold text-gray-900 dark:text-white">Custom Formatting (23pt / 16pt / 14pt)</div>
                  <div class="text-xs text-gray-500 mt-0.5">Century Gothic 23pt Project, Times New Roman 16pt Task, 14pt Subheading & Body.</div>
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- ━━━ STEP 2: Select Tasks ━━━ -->
        <div v-else-if="currentStep === 2" class="space-y-6">
          <div class="flex items-center justify-between">
            <div>
              <h2 class="text-xl font-semibold text-gray-900 dark:text-white mb-1">Select Tasks</h2>
              <p class="text-sm text-gray-500 dark:text-gray-400">
                Choose tasks from {{ monthLabel }} {{ selectedYear }} to include in the report.
              </p>
            </div>
            <div class="text-sm text-gray-500 dark:text-gray-400 font-medium">
              {{ selectedTaskIds.length }} / {{ monthTasks.length }} selected
            </div>
          </div>

          <!-- Loading state -->
          <div v-if="tasksLoading" class="flex items-center justify-center py-20">
            <div class="flex flex-col items-center gap-3 text-gray-400">
              <div class="w-8 h-8 border-2 border-primary-500 border-t-transparent rounded-full animate-spin" />
              <span class="text-sm">Loading tasks…</span>
            </div>
          </div>

          <!-- No tasks -->
          <div v-else-if="monthTasks.length === 0" class="text-center py-16 bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800">
            <FileText class="w-12 h-12 mx-auto text-gray-300 dark:text-gray-600 mb-3" />
            <p class="text-gray-500 dark:text-gray-400 font-medium">No tasks found for {{ monthLabel }} {{ selectedYear }}</p>
            <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">Add tasks in the Tasks view first.</p>
          </div>

          <!-- Task list -->
          <template v-else>
            <!-- Select all -->
            <div class="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 px-5 py-3 flex items-center gap-3">
              <input
                type="checkbox"
                :checked="allSelected"
                class="w-4 h-4 rounded text-primary-600 focus:ring-primary-500 cursor-pointer"
                @change="toggleSelectAll"
              />
              <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ allSelected ? 'Deselect All' : 'Select All' }}
              </span>
            </div>

            <!-- Per-project groups -->
            <div
              v-for="project in Object.keys(tasksByProject).sort()"
              :key="project"
              class="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 overflow-hidden"
            >
              <!-- Project header -->
              <div class="flex items-center gap-3 px-5 py-3 bg-gray-50 dark:bg-gray-800/60 border-b border-gray-200 dark:border-gray-700">
                <input
                  type="checkbox"
                  :checked="projectAllSelected(project)"
                  class="w-4 h-4 rounded text-primary-600 focus:ring-primary-500 cursor-pointer"
                  @change="toggleProjectAll(project)"
                />
                <span class="text-sm font-semibold text-gray-800 dark:text-gray-200">{{ project }}</span>
                <span class="ml-auto text-xs text-gray-400 dark:text-gray-500">
                  {{ (tasksByProject[project] ?? []).filter(t => selectedTaskIds.includes(t.id)).length }}
                  / {{ (tasksByProject[project] ?? []).length }}
                </span>
              </div>

              <!-- Task rows -->
              <div class="divide-y divide-gray-100 dark:divide-gray-800">
                <div
                  v-for="task in tasksByProject[project]"
                  :key="task.id"
                  class="flex items-start gap-3 px-5 py-3 hover:bg-gray-50 dark:hover:bg-gray-800/40 cursor-pointer transition-colors"
                  @click="toggleTask(task.id)"
                >
                  <input
                    type="checkbox"
                    :checked="isTaskSelected(task.id)"
                    class="w-4 h-4 rounded text-primary-600 focus:ring-primary-500 cursor-pointer mt-0.5"
                    @click.stop
                    @change="toggleTask(task.id)"
                  />
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="text-sm font-medium text-gray-900 dark:text-white leading-snug">{{ task.title }}</span>
                      <span
                        class="text-xs px-1.5 py-0.5 rounded-full font-medium"
                        :class="{
                          'bg-green-100 dark:bg-green-900/40 text-green-700 dark:text-green-300': task.status === 'completed',
                          'bg-blue-100 dark:bg-blue-900/40 text-blue-700 dark:text-blue-300': task.status === 'in-progress',
                          'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400': task.status === 'pending',
                        }"
                      >
                        {{ task.status }}
                      </span>
                    </div>
                    <div class="flex items-center gap-2 mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                      <span>{{ task.date }}</span>
                      <span v-if="task.shortDescription" class="truncate">• {{ task.shortDescription }}</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </template>
        </div>

        <!-- ━━━ STEP 3: Organize ━━━ -->
        <div v-else-if="currentStep === 3" class="space-y-6">
          <div>
            <h2 class="text-xl font-semibold text-gray-900 dark:text-white mb-1">Organize Tasks</h2>
            <p class="text-sm text-gray-500 dark:text-gray-400">Reorder tasks within each project using the arrow buttons.</p>
          </div>

          <div
            v-for="project in orderedProjects"
            :key="project"
            class="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 overflow-hidden"
          >
            <!-- Project header -->
            <div class="flex items-center px-5 py-3 bg-gray-50 dark:bg-gray-800/60 border-b border-gray-200 dark:border-gray-700">
              <span class="text-sm font-semibold text-gray-800 dark:text-gray-200">{{ project }}</span>
              <span class="ml-auto text-xs text-gray-400 dark:text-gray-500">{{ (orderedByProject[project] ?? []).length }} tasks</span>
            </div>

            <!-- Task rows with drag handle + arrows -->
            <div class="divide-y divide-gray-100 dark:divide-gray-800">
              <div
                v-for="(task, idx) in orderedByProject[project]"
                :key="task.taskId"
                class="flex items-center gap-3 px-5 py-3 group"
              >
                <!-- Drag handle (visual only) -->
                <div class="flex flex-col gap-0.5 opacity-30 group-hover:opacity-60 cursor-grab select-none">
                  <div class="w-4 h-0.5 bg-gray-500 rounded" />
                  <div class="w-4 h-0.5 bg-gray-500 rounded" />
                  <div class="w-4 h-0.5 bg-gray-500 rounded" />
                </div>

                <!-- Order badge -->
                <div class="w-6 h-6 rounded-full bg-primary-100 dark:bg-primary-900/40 text-primary-700 dark:text-primary-300 text-xs font-bold flex items-center justify-center shrink-0">
                  {{ idx + 1 }}
                </div>

                <div class="flex-1 min-w-0">
                  <p class="text-sm font-medium text-gray-900 dark:text-white truncate">{{ task.title }}</p>
                  <p class="text-xs text-gray-400 dark:text-gray-500">{{ task.date }}</p>
                </div>

                <!-- Up/Down controls -->
                <div class="flex flex-col gap-1">
                  <button
                    class="p-1 rounded hover:bg-gray-100 dark:hover:bg-gray-800 text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 disabled:opacity-20 disabled:cursor-not-allowed transition-colors"
                    :disabled="idx === 0"
                    @click="moveTaskUp(project, task.taskId)"
                    title="Move up"
                  >
                    <ChevronLeft class="w-4 h-4 rotate-90" />
                  </button>
                  <button
                    class="p-1 rounded hover:bg-gray-100 dark:hover:bg-gray-800 text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 disabled:opacity-20 disabled:cursor-not-allowed transition-colors"
                    :disabled="idx === (orderedByProject[project]?.length ?? 0) - 1"
                    @click="moveTaskDown(project, task.taskId)"
                    title="Move down"
                  >
                    <ChevronRight class="w-4 h-4 rotate-90" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- ━━━ STEP 4: Edit Content ━━━ -->
        <div v-else-if="currentStep === 4" class="space-y-8">
          <div>
            <h2 class="text-xl font-semibold text-gray-900 dark:text-white mb-1">Edit Report Content</h2>
            <p class="text-sm text-gray-500 dark:text-gray-400">Review and edit the cover page, task details, and conclusion.</p>
          </div>

          <!-- Cover page form -->
          <div class="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
            <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider mb-5">Cover Page</h3>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div class="sm:col-span-2 flex flex-col gap-1">
                <label class="text-xs text-gray-500 dark:text-gray-400 font-medium">Report Title</label>
                <input
                  v-model="coverForm.title"
                  type="text"
                  class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
                />
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-xs text-gray-500 dark:text-gray-400 font-medium">Employee Name <span class="text-red-500">*</span></label>
                <input
                  v-model="coverForm.employeeName"
                  type="text"
                  class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
                  placeholder="Your full name"
                />
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-xs text-gray-500 dark:text-gray-400 font-medium">Employee Code</label>
                <input
                  v-model="coverForm.employeeCode"
                  type="text"
                  class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
                  placeholder="EMP-001"
                />
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-xs text-gray-500 dark:text-gray-400 font-medium">Appraiser Name</label>
                <input
                  v-model="coverForm.appraiserName"
                  type="text"
                  class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
                  placeholder="Manager / Appraiser"
                />
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-xs text-gray-500 dark:text-gray-400 font-medium">Department</label>
                <input
                  v-model="coverForm.department"
                  type="text"
                  class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
                  placeholder="Engineering"
                />
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-xs text-gray-500 dark:text-gray-400 font-medium">Month / Year (Cover Display)</label>
                <input
                  v-model="coverForm.monthYear"
                  type="text"
                  class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-gray-900 dark:text-white text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
                  placeholder="October 2026"
                />
              </div>
            </div>
          </div>

          <!-- Dynamic Projects & Tasks Editor -->
          <div class="space-y-4">
            <div class="flex items-center justify-between">
              <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
                Dynamic Projects & Tasks
              </h3>
              <button
                type="button"
                @click="addProjectSection"
                class="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg bg-primary-600 hover:bg-primary-700 text-white text-xs font-semibold transition"
              >
                <Plus class="w-3.5 h-3.5" /> Add Project
              </button>
            </div>

            <div
              v-for="(sec, pIdx) in editableSections"
              :key="pIdx"
              class="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 overflow-hidden"
            >
              <div class="flex items-center gap-2 px-5 py-3 bg-gray-50 dark:bg-gray-800/60 border-b border-gray-200 dark:border-gray-700">
                <input
                  v-model="sec.project"
                  @input="syncCoverProjects"
                  type="text"
                  class="flex-1 px-2.5 py-1 rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm font-bold text-gray-900 dark:text-white"
                  placeholder="Project Name"
                />
                <button
                  type="button"
                  @click="addTaskToSection(pIdx)"
                  class="inline-flex items-center gap-1 px-2.5 py-1 rounded bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 text-xs font-medium text-primary-700 dark:text-primary-300 hover:bg-primary-50"
                >
                  <Plus class="w-3 h-3" /> Add Task
                </button>
                <button
                  type="button"
                  :disabled="pIdx === 0"
                  @click="moveProjectSection(pIdx, -1)"
                  class="p-1 rounded hover:bg-gray-200 disabled:opacity-30"
                  title="Move Project Up"
                >
                  <ArrowUp class="w-3.5 h-3.5" />
                </button>
                <button
                  type="button"
                  :disabled="pIdx === editableSections.length - 1"
                  @click="moveProjectSection(pIdx, 1)"
                  class="p-1 rounded hover:bg-gray-200 disabled:opacity-30"
                  title="Move Project Down"
                >
                  <ArrowDown class="w-3.5 h-3.5" />
                </button>
                <button
                  type="button"
                  @click="deleteProjectSection(pIdx)"
                  class="p-1 rounded text-red-500 hover:bg-red-50"
                  title="Delete Project"
                >
                  <Trash2 class="w-3.5 h-3.5" />
                </button>
              </div>

              <div class="divide-y divide-gray-100 dark:divide-gray-800">
                <div
                  v-for="(task, tIdx) in sec.tasks"
                  :key="task.taskId"
                  class="px-5 py-4"
                >
                  <div class="flex items-center gap-2">
                    <input
                      v-model="task.title"
                      type="text"
                      class="flex-1 px-3 py-1.5 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-sm font-medium text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
                      placeholder="Task title"
                    />
                    <button type="button" :disabled="tIdx === 0" @click="moveTaskInSection(pIdx, tIdx, -1)" class="p-1 rounded hover:bg-gray-100 disabled:opacity-30" title="Move Up">
                      <ArrowUp class="w-3.5 h-3.5" />
                    </button>
                    <button type="button" :disabled="tIdx === sec.tasks.length - 1" @click="moveTaskInSection(pIdx, tIdx, 1)" class="p-1 rounded hover:bg-gray-100 disabled:opacity-30" title="Move Down">
                      <ArrowDown class="w-3.5 h-3.5" />
                    </button>
                    <button type="button" @click="duplicateTaskInSection(pIdx, tIdx)" class="p-1 rounded text-gray-500 hover:bg-gray-100" title="Duplicate Task">
                      <Copy class="w-3.5 h-3.5" />
                    </button>
                    <button type="button" @click="deleteTaskInSection(pIdx, tIdx)" class="p-1 rounded text-red-500 hover:bg-red-50" title="Delete Task">
                      <Trash2 class="w-3.5 h-3.5" />
                    </button>
                    <button
                      type="button"
                      class="text-xs text-primary-600 dark:text-primary-400 hover:underline whitespace-nowrap ml-1"
                      @click="toggleExpandTask(task)"
                    >
                      {{ task.expanded ? 'Collapse' : 'Edit Details' }}
                    </button>
                  </div>

                  <div v-if="task.expanded" class="mt-3 grid grid-cols-1 gap-3">
                    <div class="flex flex-col gap-1">
                      <label class="text-xs text-gray-500 dark:text-gray-400 font-medium">Issue Identified</label>
                      <textarea v-model="task.issueIdentified" rows="2" class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-sm text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500" placeholder="Bullet points (- item) or numbered list (1. item)" />
                    </div>
                    <div class="flex flex-col gap-1">
                      <label class="text-xs text-gray-500 dark:text-gray-400 font-medium">Activities Performed</label>
                      <textarea v-model="task.activitiesPerformed" rows="3" class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-sm text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500" placeholder="- Implemented feature&#10;  o Sub-bullet detail" />
                    </div>
                    <div class="flex flex-col gap-1">
                      <label class="text-xs text-gray-500 dark:text-gray-400 font-medium">Solution Implemented</label>
                      <textarea v-model="task.solutionImplemented" rows="2" class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-sm text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500" />
                    </div>
                    <div class="flex flex-col gap-1">
                      <label class="text-xs text-gray-500 dark:text-gray-400 font-medium">Challenges Faced (Optional)</label>
                      <textarea v-model="task.challengesFaced" rows="2" class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-sm text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500" />
                    </div>
                    <div class="flex flex-col gap-1">
                      <label class="text-xs text-gray-500 dark:text-gray-400 font-medium">Outcome</label>
                      <textarea v-model="task.outcome" rows="2" class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-sm text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500" />
                    </div>

                    <!-- Optional Custom Subheadings -->
                    <div v-for="(cs, cIdx) in task.customSections" :key="cIdx" class="p-3 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 space-y-2">
                      <div class="flex items-center gap-2">
                        <input v-model="cs.heading" type="text" class="flex-1 px-2.5 py-1 rounded border text-xs font-semibold" placeholder="Subheading Title" />
                        <button type="button" @click="removeCustomSubheading(task, cIdx)" class="text-red-500 text-xs hover:underline">Remove</button>
                      </div>
                      <textarea v-model="cs.content" rows="2" class="w-full px-2.5 py-1.5 rounded border text-xs" placeholder="Bullet points or description" />
                    </div>
                    <div>
                      <button type="button" @click="addCustomSubheading(task)" class="text-xs text-primary-600 hover:underline flex items-center gap-1">
                        <Plus class="w-3 h-3" /> Add Optional Subheading
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Conclusion -->
          <div class="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
            <div class="flex items-center gap-2 mb-4">
              <Sparkles class="w-4 h-4 text-primary-500" />
              <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">Conclusion</h3>
              <span class="text-xs text-gray-400 dark:text-gray-500 italic">(auto-generated, editable)</span>
            </div>
            <textarea
              v-model="conclusionText"
              rows="5"
              class="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-sm text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500 resize-y"
              placeholder="Summary of this month's performance…"
            />
          </div>
        </div>

        <!-- ━━━ STEP 5: Preview & Edit Document ━━━ -->
        <div v-else-if="currentStep === 5" class="space-y-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h2 class="text-xl font-semibold text-gray-900 dark:text-white mb-1">
                {{ step5Mode === 'edit' ? 'Edit Document' : 'Preview Report' }}
              </h2>
              <p class="text-sm text-gray-500 dark:text-gray-400">
                {{
                  step5Mode === 'edit'
                    ? 'Edit text, alignment, spacing, page breaks, images, tables, and margins directly.'
                    : 'Review the paginated layout, automatic Table of Contents, and formatting before downloading.'
                }}
              </p>
            </div>
            <div class="flex items-center gap-2 flex-wrap">
              <!-- Mode Toggle: [Preview Report] [Edit Document] -->
              <div class="inline-flex rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-900 p-0.5">
                <button
                  :class="[
                    'flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-semibold transition-colors',
                    step5Mode === 'preview'
                      ? 'bg-primary-600 text-white shadow-sm'
                      : 'text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800'
                  ]"
                  @click="step5Mode = 'preview'"
                >
                  <Eye class="w-3.5 h-3.5" />
                  <span>Preview Report</span>
                </button>
                <button
                  :class="[
                    'flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-semibold transition-colors',
                    step5Mode === 'edit'
                      ? 'bg-primary-600 text-white shadow-sm'
                      : 'text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800'
                  ]"
                  @click="step5Mode = 'edit'"
                >
                  <Pencil class="w-3.5 h-3.5" />
                  <span>Edit Document</span>
                </button>
              </div>

              <template v-if="step5Mode === 'preview'">
                <div class="flex items-center gap-1 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-lg px-2 py-1">
                  <button @click="previewZoom = Math.max(0.5, Number((previewZoom - 0.1).toFixed(2)))" class="p-1 hover:bg-gray-100 rounded" title="Zoom Out"><ZoomOut class="w-4 h-4" /></button>
                  <span class="text-xs font-mono px-1.5">{{ Math.round(previewZoom * 100) }}%</span>
                  <button @click="previewZoom = Math.min(1.5, Number((previewZoom + 0.1).toFixed(2)))" class="p-1 hover:bg-gray-100 rounded" title="Zoom In"><ZoomIn class="w-4 h-4" /></button>
                </div>
                <button
                  class="px-3 py-2 rounded-lg border border-gray-300 text-xs font-medium hover:bg-gray-50"
                  @click="currentStep = 4"
                >
                  Edit Content
                </button>
                <button
                  class="flex items-center gap-2 px-4 py-2 rounded-lg border border-primary-300 dark:border-primary-700 text-primary-700 dark:text-primary-300 text-sm font-medium hover:bg-primary-50 dark:hover:bg-primary-900/20 transition-colors disabled:opacity-50"
                  :disabled="loading"
                  @click="regeneratePreview"
                >
                  <Sparkles class="w-4 h-4" />
                  <span>{{ loading ? 'Generating…' : 'Regenerate Preview' }}</span>
                </button>
              </template>

              <button
                class="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-red-600 hover:bg-red-700 text-white text-xs font-semibold shadow-sm"
                @click="downloadPdf"
              >
                <Download class="w-3.5 h-3.5" />
                <span>Download PDF</span>
              </button>
              <button
                class="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-blue-600 hover:bg-blue-700 text-white text-xs font-semibold shadow-sm"
                @click="downloadDocx"
              >
                <Download class="w-3.5 h-3.5" />
                <span>Download DOCX</span>
              </button>
              <button
                v-if="monthStatus?.status !== 'COMPLETED'"
                class="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-green-600 hover:bg-green-700 text-white text-xs font-semibold shadow-sm disabled:opacity-50"
                :disabled="completingReport"
                @click="markAsCompleted"
              >
                <Check class="w-3.5 h-3.5" />
                <span>{{ completingReport ? 'Finalizing…' : 'Mark as Completed' }}</span>
              </button>
              <span v-else class="px-3 py-2 rounded-lg bg-green-100 text-green-800 border border-green-200 text-xs font-bold">
                COMPLETED
              </span>
            </div>
          </div>

          <!-- Edited Document / Task Changed Protection Modal -->
          <div v-if="showRegenChoiceModal" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
            <div class="bg-white rounded-2xl shadow-2xl max-w-md w-full p-6 space-y-4">
              <h3 class="text-base font-bold text-gray-900">Existing Report / Manual Edits Detected</h3>
              <p class="text-sm text-gray-600">
                Your task data has changed since this report was generated, or manual document edits exist. Would you like to update the report? (A backup of your edited document will be saved before regenerating.)
              </p>
              <div class="flex flex-wrap justify-end gap-2 pt-2">
                <button
                  class="px-3 py-2 rounded-lg border border-gray-300 text-xs font-medium hover:bg-gray-50"
                  @click="showRegenChoiceModal = false"
                >
                  Cancel
                </button>
                <button
                  class="px-3 py-2 rounded-lg border border-primary-300 text-primary-700 text-xs font-semibold hover:bg-primary-50"
                  @click="showRegenChoiceModal = false; previewUrl = reportsApi.previewUrl(yearStr, monthPadded) + '?t=' + Date.now(); previewKey++"
                >
                  Keep Existing Report
                </button>
                <button
                  class="px-3 py-2 rounded-lg bg-primary-600 text-white text-xs font-semibold hover:bg-primary-700"
                  @click="showRegenChoiceModal = false; generateAndPreview(true)"
                >
                  Regenerate Report
                </button>
              </div>
            </div>
          </div>

          <!-- Ambiguous Headings / Descriptions Notice -->
          <div v-if="ambiguousWarning" class="bg-amber-50 border border-amber-200 rounded-xl px-4 py-3 flex items-center justify-between gap-3 text-xs text-amber-900">
            <div>
              <strong>Please Review Uncertain Headings/Descriptions:</strong> {{ ambiguousWarning }}
            </div>
            <div class="flex items-center gap-2 shrink-0">
              <button v-if="step5Mode !== 'edit'" class="px-2.5 py-1 rounded bg-amber-600 text-white font-semibold hover:bg-amber-700" @click="step5Mode = 'edit'">
                Fix in Edit Document
              </button>
              <button class="text-amber-700 hover:underline" @click="ambiguousWarning = ''">Dismiss</button>
            </div>
          </div>

          <!-- Separate Word-like Document Editor -->
          <DocumentEditor
            v-if="step5Mode === 'edit'"
            :year="yearStr"
            :month="monthPadded"
            @saved="onEditorSaved"
            @status-change="editorStatus = $event"
          />

          <!-- Loading overlay -->
          <div v-else-if="loading" class="flex items-center justify-center py-24 bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800">
            <div class="flex flex-col items-center gap-4 text-gray-400">
              <div class="w-10 h-10 border-2 border-primary-500 border-t-transparent rounded-full animate-spin" />
              <span class="text-sm">Generating report, please wait…</span>
            </div>
          </div>

          <!-- Existing iframe PDF Preview (unchanged) -->
          <div v-else-if="previewUrl" class="bg-gray-200 dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 overflow-auto" style="height: 70vh;">
            <div :style="{ transform: `scale(${previewZoom})`, transformOrigin: 'top center', height: `${100 / previewZoom}%` }">
              <iframe
                :key="previewKey"
                :src="previewUrl"
                class="w-full h-full border-0"
                title="Report Preview"
              />
            </div>
          </div>

          <!-- No preview yet -->
          <div v-else class="flex flex-col items-center justify-center py-24 bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 gap-4">
            <Eye class="w-14 h-14 text-gray-300 dark:text-gray-600" />
            <div class="text-center">
              <p class="text-gray-500 dark:text-gray-400 font-medium">No preview available</p>
              <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">Click "Regenerate Preview" to create one.</p>
            </div>
          </div>
        </div>

        <!-- ━━━ STEP 6: Download ━━━ -->
        <div v-else-if="currentStep === 6" class="space-y-8">
          <div class="text-center">
            <div class="w-16 h-16 bg-green-100 dark:bg-green-900/30 rounded-full flex items-center justify-center mx-auto mb-4">
              <Check class="w-8 h-8 text-green-600 dark:text-green-400" />
            </div>
            <h2 class="text-2xl font-bold text-gray-900 dark:text-white mb-2">Report Ready!</h2>
            <p class="text-gray-500 dark:text-gray-400">
              Your performance report for <strong>{{ monthLabel }} {{ selectedYear }}</strong> has been generated.
              Download it in your preferred format below.
            </p>
          </div>

          <!-- Optional custom save folder -->
          <div class="max-w-2xl mx-auto bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-4">
            <label class="block text-xs font-semibold text-gray-600 dark:text-gray-400 mb-1">Optional Local Save Directory (leave blank for browser Downloads folder)</label>
            <input
              v-model="downloadDir"
              type="text"
              class="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-xs font-mono"
              placeholder="e.g. D:\Reports\2026"
            />
          </div>

          <!-- Download buttons -->
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-6 max-w-2xl mx-auto">
            <!-- PDF -->
            <button
              class="group flex flex-col items-center gap-4 p-8 bg-white dark:bg-gray-900 rounded-2xl border-2 border-gray-200 dark:border-gray-800 hover:border-red-400 dark:hover:border-red-600 hover:shadow-lg transition-all duration-200"
              @click="downloadPdf"
            >
              <div class="w-14 h-14 bg-red-100 dark:bg-red-900/30 rounded-xl flex items-center justify-center group-hover:scale-110 transition-transform">
                <Download class="w-7 h-7 text-red-600 dark:text-red-400" />
              </div>
              <div class="text-center">
                <p class="font-semibold text-gray-900 dark:text-white text-lg">Download PDF</p>
                <p class="text-xs text-gray-400 dark:text-gray-500 mt-1 font-mono break-all">{{ pdfFilename }}</p>
              </div>
            </button>

            <!-- DOCX -->
            <button
              class="group flex flex-col items-center gap-4 p-8 bg-white dark:bg-gray-900 rounded-2xl border-2 border-gray-200 dark:border-gray-800 hover:border-blue-400 dark:hover:border-blue-600 hover:shadow-lg transition-all duration-200"
              @click="downloadDocx"
            >
              <div class="w-14 h-14 bg-blue-100 dark:bg-blue-900/30 rounded-xl flex items-center justify-center group-hover:scale-110 transition-transform">
                <Download class="w-7 h-7 text-blue-600 dark:text-blue-400" />
              </div>
              <div class="text-center">
                <p class="font-semibold text-gray-900 dark:text-white text-lg">Download Word</p>
                <p class="text-xs text-gray-400 dark:text-gray-500 mt-1 font-mono break-all">{{ docxFilename }}</p>
              </div>
            </button>
          </div>

          <!-- Report meta -->
          <div v-if="reportData" class="max-w-2xl mx-auto bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-5">
            <h3 class="text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-3">Report Details</h3>
            <div class="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
              <div class="text-gray-500 dark:text-gray-400">Version</div>
              <div class="text-gray-900 dark:text-white font-medium">v{{ reportData.version }}</div>
              <div class="text-gray-500 dark:text-gray-400">Status</div>
              <div>
                <span
                  class="text-xs px-2 py-0.5 rounded-full font-medium"
                  :class="reportData.status === 'final' ? 'bg-green-100 dark:bg-green-900/40 text-green-700 dark:text-green-300' : 'bg-yellow-100 dark:bg-yellow-900/40 text-yellow-700 dark:text-yellow-300'"
                >
                  {{ reportData.status }}
                </span>
              </div>
              <div class="text-gray-500 dark:text-gray-400">Tasks Included</div>
              <div class="text-gray-900 dark:text-white font-medium">{{ selectedTaskIds.length }}</div>
              <div class="text-gray-500 dark:text-gray-400">Projects</div>
              <div class="text-gray-900 dark:text-white font-medium">{{ orderedProjects.length }}</div>
              <div class="text-gray-500 dark:text-gray-400">Generated At</div>
              <div class="text-gray-900 dark:text-white font-medium">
                {{ reportData.generatedAt ? format(new Date(reportData.generatedAt), 'MMM d, yyyy HH:mm') : '—' }}
              </div>
            </div>
          </div>

          <!-- New version -->
          <div class="text-center">
            <button
              class="inline-flex items-center gap-2 px-5 py-2.5 rounded-lg border border-gray-300 dark:border-gray-700 text-gray-700 dark:text-gray-300 text-sm font-medium hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors"
              @click="startNewVersion"
            >
              <Sparkles class="w-4 h-4" />
              Generate New Version
            </button>
          </div>
        </div>

      </div>
    </div>

    <!-- ── Bottom Navigation ── -->
    <div class="bg-white dark:bg-gray-900 border-t border-gray-200 dark:border-gray-800 px-6 py-4">
      <div class="max-w-5xl mx-auto flex items-center justify-between">
        <button
          v-if="currentStep > 1"
          class="flex items-center gap-2 px-5 py-2.5 rounded-lg border border-gray-300 dark:border-gray-700 text-gray-700 dark:text-gray-300 text-sm font-medium hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors"
          @click="prevStep"
        >
          <ChevronLeft class="w-4 h-4" />
          Previous
        </button>
        <div v-else />

        <div class="flex items-center gap-2">
          <!-- Progress dots -->
          <div
            v-for="step in STEPS"
            :key="step.number"
            class="w-2 h-2 rounded-full transition-colors"
            :class="step.number === currentStep ? 'bg-primary-600' : step.number < currentStep ? 'bg-primary-300 dark:bg-primary-700' : 'bg-gray-300 dark:bg-gray-700'"
          />
        </div>

        <button
          v-if="currentStep < 6"
          class="flex items-center gap-2 px-5 py-2.5 rounded-lg bg-primary-600 hover:bg-primary-700 text-white text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          :disabled="loading"
          @click="nextStep"
        >
          <span>{{ currentStep === 4 ? (loading ? 'Generating…' : 'Generate & Preview') : 'Next' }}</span>
          <ChevronRight class="w-4 h-4" />
        </button>
        <div v-else />
      </div>
    </div>
  </div>
</template>
