<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useTasksStore } from '@/stores/tasks'
import { useProfileStore } from '@/stores/profile'
import { useUIStore } from '@/stores/ui'
import { format } from 'date-fns'
import type { Task, Template } from '@/types'
import { reportsApi, templatesApi, editorApi } from '@/api'
import DocumentEditor from '@/components/DocumentEditor.vue'
import {
  Search, Pencil, Trash2, Copy, Download, X, Check,
  Calendar, Filter, ChevronDown, ListFilter, Eye, FileText, Sparkles
} from 'lucide-vue-next'

const tasksStore = useTasksStore()
const profileStore = useProfileStore()
const uiStore = useUIStore()

const currentMonth = ref(format(new Date(), 'yyyy-MM'))
const searchQuery = ref('')
const filterProject = ref('')
const filterStatus = ref('')
const groupBy = ref<'project' | 'date' | 'status'>('project')
const selectedIds = ref<Set<string>>(new Set())
const showEditModal = ref(false)
const editingTask = ref<Task | null>(null)

// Month year options
const monthOptions = Array.from({ length: 12 }, (_, i) => {
  const d = new Date(2026, i, 1)
  return { label: format(d, 'MMMM'), value: String(i + 1).padStart(2, '0') }
})
const yearOptions = ['2024', '2025', '2026', '2027']
const [selYear, selMonth] = currentMonth.value.split('-')
const selectedYear = ref(selYear)
const selectedMonth = ref(selMonth)

watch([selectedYear, selectedMonth], ([y, m]) => {
  currentMonth.value = `${y}-${m}`
  tasksStore.fetchTasks(currentMonth.value)
})

const filteredTasks = computed(() => {
  let list = tasksStore.tasks
  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase()
    list = list.filter(t =>
      t.title.toLowerCase().includes(q) ||
      t.shortDescription?.toLowerCase().includes(q) ||
      t.project?.toLowerCase().includes(q)
    )
  }
  if (filterProject.value) list = list.filter(t => t.project === filterProject.value)
  if (filterStatus.value) list = list.filter(t => t.status === filterStatus.value)
  return list
})

const grouped = computed(() => {
  const groups: Record<string, Task[]> = {}
  for (const t of filteredTasks.value) {
    const key = groupBy.value === 'project' ? (t.project || 'No Project')
      : groupBy.value === 'status' ? (t.status || 'pending')
      : t.date
    if (!groups[key]) groups[key] = []
    groups[key].push(t)
  }
  return Object.entries(groups).sort(([a], [b]) => a.localeCompare(b))
})

const projectsList = computed(() => {
  const s = new Set(tasksStore.tasks.map(t => t.project).filter(Boolean))
  return Array.from(s)
})

const stats = computed(() => {
  const all = tasksStore.tasks
  return {
    total: all.length,
    completed: all.filter(t => t.status === 'completed').length,
    inProgress: all.filter(t => t.status === 'in-progress').length,
    pending: all.filter(t => t.status === 'pending').length,
  }
})

function toggleSelect(id: string) {
  if (selectedIds.value.has(id)) selectedIds.value.delete(id)
  else selectedIds.value.add(id)
}
function selectAll() {
  filteredTasks.value.forEach(t => selectedIds.value.add(t.id))
}
function deselectAll() { selectedIds.value.clear() }
function isSelected(id: string) { return selectedIds.value.has(id) }

function editTask(task: Task) {
  editingTask.value = { ...task }
  showEditModal.value = true
}

async function saveEdit() {
  if (!editingTask.value) return
  await tasksStore.updateTask(editingTask.value.id, editingTask.value)
  uiStore.notify('Task updated successfully')
  showEditModal.value = false
}

async function deleteTask(task: Task) {
  uiStore.confirm('Delete Task', `Delete "${task.title}"? This cannot be undone.`, async () => {
    await tasksStore.deleteTask(task.id)
    uiStore.notify('Task deleted')
  })
}

async function duplicateTask(task: Task) {
  const copy = { ...task, id: '', title: task.title + ' (Copy)', createdAt: '', updatedAt: '' }
  await tasksStore.createTask(copy)
  uiStore.notify('Task duplicated')
}

async function deleteSelected() {
  uiStore.confirm('Delete Selected', `Delete ${selectedIds.value.size} tasks?`, async () => {
    for (const id of selectedIds.value) {
      await tasksStore.deleteTask(id)
    }
    selectedIds.value.clear()
    uiStore.notify('Tasks deleted')
  })
}

function exportCSV() {
  const rows = [['Date', 'Project', 'Title', 'Short Description', 'Status', 'Tags']]
  filteredTasks.value.forEach(t => {
    rows.push([t.date, t.project, t.title, t.shortDescription, t.status, (t.tags || []).join(';')])
  })
  const csv = rows.map(r => r.map(c => `"${(c || '').replace(/"/g, '""')}"`).join(',')).join('\n')
  const blob = new Blob([csv], { type: 'text/csv' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `tasks-${currentMonth.value}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

const statusClass: Record<string, string> = {
  completed: 'badge-status-completed',
  'in-progress': 'badge-status-in-progress',
  pending: 'badge-status-pending',
}

const viewMode = ref<'list' | 'report'>('list')
const reportSubMode = ref<'preview' | 'edit'>('preview')
const reportLoading = ref(false)
const reportPreviewUrl = ref('')
const reportPreviewKey = ref(0)
const templates = ref<Template[]>([])
const selectedTemplateId = ref('era')

async function syncReportView(forceRegen = false) {
  reportLoading.value = true
  try {
    let skipRegen = false
    if (!forceRegen) {
      try {
        const st = await editorApi.status(selectedYear.value, selectedMonth.value)
        if (st.hasSavedEdits || st.hasDraft) {
          skipRegen = true
        }
      } catch {
        // ignore
      }
    } else {
      try {
        const st = await editorApi.status(selectedYear.value, selectedMonth.value)
        if (st.hasSavedEdits || st.hasDraft) {
          const ok = window.confirm('Manual edits exist for this month. Refreshing from selected tasks will replace manual edits (a backup will be saved). Continue?')
          if (!ok) {
            skipRegen = true
          }
        }
      } catch {
        // ignore
      }
    }

    if (!skipRegen) {
      const ids = selectedIds.value.size > 0
        ? Array.from(selectedIds.value)
        : filteredTasks.value.map(t => t.id)
      const tmpl = templates.value.find(t => t.id === selectedTemplateId.value)
      await reportsApi.generate(
        currentMonth.value,
        ids,
        selectedTemplateId.value || 'era',
        tmpl?.formattingMode || 'match_template',
        forceRegen
      )
    }
    reportPreviewUrl.value = reportsApi.previewUrl(selectedYear.value, selectedMonth.value) + '?t=' + Date.now()
    reportPreviewKey.value++
  } catch (e: any) {
    uiStore.notify('Failed to render Report View: ' + (e?.message || 'Unknown error'), 'error')
  } finally {
    reportLoading.value = false
  }
}

async function switchToReportView() {
  viewMode.value = 'report'
  reportSubMode.value = 'preview'
  await syncReportView(false)
}

function onMyTasksEditorSaved() {
  reportPreviewUrl.value = reportsApi.previewUrl(selectedYear.value, selectedMonth.value) + '?t=' + Date.now()
  reportPreviewKey.value++
  reportSubMode.value = 'preview'
}

onMounted(async () => {
  await tasksStore.fetchTasks(currentMonth.value)
  try {
    templates.value = await templatesApi.list()
    const def = templates.value.find(t => t.isDefault) || templates.value[0]
    if (def) selectedTemplateId.value = def.id
  } catch {
    templates.value = []
  }
})
</script>

<template>
  <div class="min-h-full bg-gray-50 p-8 animate-fade-in">
    <!-- Header -->
    <div class="flex items-center justify-between mb-6">
      <div>
        <h1 class="text-2xl font-bold text-gray-900">My Tasks</h1>
        <p class="text-sm text-gray-500 mt-0.5">View and manage all your logged tasks</p>
      </div>
      <div class="flex items-center gap-3 flex-wrap">
        <div class="inline-flex rounded-lg border border-gray-300 bg-white p-0.5">
          <button
            @click="viewMode = 'list'"
            :class="[
              'flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-semibold transition-colors',
              viewMode === 'list' ? 'bg-primary-600 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100'
            ]"
          >
            <ListFilter class="w-3.5 h-3.5" />
            <span>Task List</span>
          </button>
          <button
            @click="switchToReportView"
            :class="[
              'flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-semibold transition-colors',
              viewMode === 'report' ? 'bg-primary-600 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100'
            ]"
          >
            <FileText class="w-3.5 h-3.5" />
            <span>Report View</span>
          </button>
        </div>
        <select v-model="selectedMonth" class="input-field w-auto text-sm">
          <option v-for="m in monthOptions" :key="m.value" :value="m.value">{{ m.label }}</option>
        </select>
        <select v-model="selectedYear" class="input-field w-auto text-sm">
          <option v-for="y in yearOptions" :key="y" :value="y">{{ y }}</option>
        </select>
        <button @click="exportCSV" class="btn-secondary flex items-center gap-2 text-sm">
          <Download class="w-4 h-4" /> Export CSV
        </button>
      </div>
    </div>

    <!-- REPORT VIEW -->
    <div v-if="viewMode === 'report'" class="space-y-4">
      <div class="card py-3 px-4 flex flex-wrap items-center justify-between gap-3">
        <div class="flex items-center gap-3 flex-wrap">
          <label class="text-xs font-semibold text-gray-600">Template:</label>
          <select v-model="selectedTemplateId" class="input-field w-auto text-xs py-1.5" @change="syncReportView(true)">
            <option v-for="t in templates" :key="t.id" :value="t.id">{{ t.name }}</option>
          </select>
          <span class="text-xs text-gray-500">
            {{ selectedIds.size > 0 ? `Using ${selectedIds.size} selected task(s)` : `Using all ${filteredTasks.length} task(s)` }}
          </span>
        </div>

        <div class="flex items-center gap-2 flex-wrap">
          <div class="inline-flex rounded-lg border border-gray-300 bg-white p-0.5">
            <button
              @click="reportSubMode = 'preview'"
              :class="[
                'flex items-center gap-1.5 px-3 py-1 rounded-md text-xs font-semibold',
                reportSubMode === 'preview' ? 'bg-primary-600 text-white' : 'text-gray-600 hover:bg-gray-100'
              ]"
            >
              <Eye class="w-3.5 h-3.5" /> Formatted Report
            </button>
            <button
              @click="reportSubMode = 'edit'"
              :class="[
                'flex items-center gap-1.5 px-3 py-1 rounded-md text-xs font-semibold',
                reportSubMode === 'edit' ? 'bg-primary-600 text-white' : 'text-gray-600 hover:bg-gray-100'
              ]"
            >
              <Pencil class="w-3.5 h-3.5" /> Word-Style Editor
            </button>
          </div>
          <button @click="syncReportView(true)" :disabled="reportLoading" class="btn-secondary text-xs py-1.5 px-3 flex items-center gap-1.5">
            <Sparkles class="w-3.5 h-3.5" /> Refresh from Tasks
          </button>
          <a :href="reportsApi.exportPdfUrl(selectedYear, selectedMonth)" target="_blank" class="px-3 py-1.5 rounded-lg bg-red-600 hover:bg-red-700 text-white text-xs font-semibold flex items-center gap-1">
            <Download class="w-3.5 h-3.5" /> PDF
          </a>
          <a :href="reportsApi.exportDocxUrl(selectedYear, selectedMonth)" target="_blank" class="px-3 py-1.5 rounded-lg bg-blue-600 hover:bg-blue-700 text-white text-xs font-semibold flex items-center gap-1">
            <Download class="w-3.5 h-3.5" /> DOCX
          </a>
        </div>
      </div>

      <DocumentEditor
        v-if="reportSubMode === 'edit'"
        :year="selectedYear"
        :month="selectedMonth"
        @saved="onMyTasksEditorSaved"
      />

      <div v-else-if="reportLoading" class="card text-center py-20 text-gray-400">
        <div class="animate-spin w-8 h-8 border-4 border-primary-500 border-t-transparent rounded-full mx-auto mb-3"></div>
        Rendering formatted monthly report...
      </div>

      <div v-else-if="reportPreviewUrl" class="bg-gray-200 rounded-xl border border-gray-200 overflow-hidden" style="height: 72vh;">
        <iframe :key="reportPreviewKey" :src="reportPreviewUrl" class="w-full h-full border-0" title="My Tasks Report View" />
      </div>
    </div>

    <!-- TASK LIST VIEW -->
    <template v-else>
    <!-- Stats Bar -->
    <div class="grid grid-cols-4 gap-4 mb-6">
      <div class="card py-3 px-4 text-center border-l-4 border-l-gray-400">
        <div class="text-2xl font-bold text-gray-900">{{ stats.total }}</div>
        <div class="text-xs text-gray-500 mt-0.5">Total Tasks</div>
      </div>
      <div class="card py-3 px-4 text-center border-l-4 border-l-green-500">
        <div class="text-2xl font-bold text-green-700">{{ stats.completed }}</div>
        <div class="text-xs text-gray-500 mt-0.5">Completed</div>
      </div>
      <div class="card py-3 px-4 text-center border-l-4 border-l-blue-500">
        <div class="text-2xl font-bold text-blue-700">{{ stats.inProgress }}</div>
        <div class="text-xs text-gray-500 mt-0.5">In Progress</div>
      </div>
      <div class="card py-3 px-4 text-center border-l-4 border-l-yellow-500">
        <div class="text-2xl font-bold text-yellow-700">{{ stats.pending }}</div>
        <div class="text-xs text-gray-500 mt-0.5">Pending</div>
      </div>
    </div>

    <!-- Filter Bar -->
    <div class="card mb-5 py-3 px-4">
      <div class="flex flex-wrap items-center gap-3">
        <div class="relative flex-1 min-w-48">
          <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
          <input v-model="searchQuery" type="text" placeholder="Search tasks..." class="input-field pl-9 text-sm" />
        </div>
        <select v-model="filterProject" class="input-field w-auto text-sm">
          <option value="">All Projects</option>
          <option v-for="p in projectsList" :key="p" :value="p">{{ p }}</option>
        </select>
        <select v-model="filterStatus" class="input-field w-auto text-sm">
          <option value="">All Status</option>
          <option value="completed">Completed</option>
          <option value="in-progress">In Progress</option>
          <option value="pending">Pending</option>
        </select>
        <div class="flex items-center gap-1 bg-gray-100 rounded-lg p-1">
          <button v-for="g in ['project', 'date', 'status']" :key="g"
            @click="groupBy = g as any"
            class="px-3 py-1.5 rounded-md text-xs font-medium transition-all"
            :class="groupBy === g ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-700'"
          >
            Group by {{ g.charAt(0).toUpperCase() + g.slice(1) }}
          </button>
        </div>
      </div>
    </div>

    <!-- Bulk Actions -->
    <Transition name="slide-up">
      <div v-if="selectedIds.size > 0" class="card mb-4 py-3 px-4 bg-primary-50 border border-primary-200">
        <div class="flex items-center justify-between">
          <span class="text-sm font-medium text-primary-700">{{ selectedIds.size }} task(s) selected</span>
          <div class="flex gap-2">
            <button @click="deselectAll" class="btn-ghost text-sm py-1.5">Deselect All</button>
            <button @click="deleteSelected" class="btn-danger text-sm py-1.5">Delete Selected</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Task Groups -->
    <div v-if="tasksStore.loading" class="text-center py-16 text-gray-400">
      <div class="animate-spin w-8 h-8 border-4 border-primary-500 border-t-transparent rounded-full mx-auto mb-3"></div>
      Loading tasks...
    </div>

    <div v-else-if="grouped.length === 0" class="card text-center py-16">
      <div class="text-5xl mb-3">📋</div>
      <p class="text-gray-500 font-medium">No tasks found</p>
      <p class="text-sm text-gray-400 mt-1">Try adjusting your filters or add some tasks</p>
    </div>

    <div v-else class="space-y-4">
      <div v-for="[groupKey, groupTasks] in grouped" :key="groupKey" class="card overflow-hidden p-0">
        <!-- Group Header -->
        <div class="flex items-center justify-between px-5 py-3 bg-gray-50 border-b border-gray-100">
          <div class="flex items-center gap-2">
            <span class="font-semibold text-gray-800 text-sm">{{ groupKey }}</span>
            <span class="text-xs bg-primary-100 text-primary-700 px-2 py-0.5 rounded-full font-medium">{{ groupTasks.length }}</span>
          </div>
          <button @click="groupTasks.forEach(t => toggleSelect(t.id))" class="text-xs text-gray-500 hover:text-gray-700">
            Select All
          </button>
        </div>

        <!-- Tasks -->
        <div class="divide-y divide-gray-50">
          <div
            v-for="task in groupTasks"
            :key="task.id"
            class="flex items-center gap-3 px-5 py-3.5 hover:bg-gray-50 transition-colors group"
          >
            <input
              type="checkbox"
              :checked="isSelected(task.id)"
              @change="toggleSelect(task.id)"
              class="rounded border-gray-300 text-primary-600 focus:ring-primary-500 flex-shrink-0"
            />
            <div class="text-xs text-gray-400 w-20 flex-shrink-0 font-mono">{{ task.date }}</div>
            <span class="text-xs px-2 py-0.5 rounded-full bg-primary-100 text-primary-700 font-medium flex-shrink-0 max-w-24 truncate">
              {{ task.project || '—' }}
            </span>
            <div class="flex-1 min-w-0">
              <p class="text-sm font-medium text-gray-900 truncate">{{ task.title }}</p>
              <p v-if="task.shortDescription" class="text-xs text-gray-400 truncate mt-0.5">{{ task.shortDescription }}</p>
            </div>
            <div class="flex items-center gap-2 flex-shrink-0">
              <span :class="statusClass[task.status]" class="hidden sm:inline-flex">{{ task.status }}</span>
              <div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                <button @click="editTask(task)" class="p-1.5 hover:bg-primary-100 rounded-lg text-primary-600 transition-colors" title="Edit">
                  <Pencil class="w-3.5 h-3.5" />
                </button>
                <button @click="duplicateTask(task)" class="p-1.5 hover:bg-blue-100 rounded-lg text-blue-600 transition-colors" title="Duplicate">
                  <Copy class="w-3.5 h-3.5" />
                </button>
                <button @click="deleteTask(task)" class="p-1.5 hover:bg-red-100 rounded-lg text-red-600 transition-colors" title="Delete">
                  <Trash2 class="w-3.5 h-3.5" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Edit Modal -->
    <Transition name="fade">
      <div v-if="showEditModal && editingTask" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
        <div class="bg-white rounded-2xl shadow-2xl w-full max-w-2xl mx-4 max-h-[90vh] overflow-y-auto">
          <div class="flex items-center justify-between p-6 border-b border-gray-100">
            <h3 class="text-lg font-bold text-gray-900">Edit Task</h3>
            <button @click="showEditModal = false" class="p-2 hover:bg-gray-100 rounded-lg">
              <X class="w-5 h-5" />
            </button>
          </div>
          <div class="p-6 space-y-4">
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="label">Project</label>
                <input v-model="editingTask.project" class="input-field" placeholder="Project name" list="edit-projects" />
                <datalist id="edit-projects">
                  <option v-for="p in profileStore.profile?.projects" :key="p" :value="p" />
                </datalist>
              </div>
              <div>
                <label class="label">Status</label>
                <select v-model="editingTask.status" class="input-field">
                  <option value="pending">Pending</option>
                  <option value="in-progress">In Progress</option>
                  <option value="completed">Completed</option>
                </select>
              </div>
            </div>
            <div>
              <label class="label">Title *</label>
              <input v-model="editingTask.title" class="input-field" required />
            </div>
            <div>
              <label class="label">Short Description</label>
              <textarea v-model="editingTask.shortDescription" class="textarea-field" rows="2" />
            </div>
            <div>
              <label class="label">Issue Identified</label>
              <textarea v-model="editingTask.issueIdentified" class="textarea-field" rows="2" />
            </div>
            <div>
              <label class="label">Activities Performed</label>
              <textarea v-model="editingTask.activitiesPerformed" class="textarea-field" rows="3" />
            </div>
            <div>
              <label class="label">Solution Implemented</label>
              <textarea v-model="editingTask.solutionImplemented" class="textarea-field" rows="2" />
            </div>
            <div>
              <label class="label">Outcome</label>
              <textarea v-model="editingTask.outcome" class="textarea-field" rows="2" />
            </div>
          </div>
          <div class="flex gap-3 justify-end p-6 border-t border-gray-100">
            <button @click="showEditModal = false" class="btn-secondary">Cancel</button>
            <button @click="saveEdit" class="btn-primary flex items-center gap-2">
              <Check class="w-4 h-4" /> Save Changes
            </button>
          </div>
        </div>
      </div>
    </Transition>
    </template>
  </div>
</template>
