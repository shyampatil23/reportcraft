<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { format, addMonths, subMonths } from 'date-fns'
import {
  Layers, Plus, Trash2, Check, ChevronLeft, ChevronRight, FolderPlus, Save, Sparkles
} from 'lucide-vue-next'
import { useTasksStore } from '@/stores/tasks'
import { useProfileStore } from '@/stores/profile'
import { useUIStore } from '@/stores/ui'
import { reportsApi } from '@/api'
import type { Task } from '@/types'

const router = useRouter()
const tasksStore = useTasksStore()
const profileStore = useProfileStore()
const uiStore = useUIStore()

interface SimpleTaskItem {
  id: string
  title: string
  description: string
}

interface SimpleProjectGroup {
  project: string // Optional (can be empty)
  tasks: SimpleTaskItem[]
}

const currentMonthDate = ref(new Date())
const currentMonthStr = computed(() => format(currentMonthDate.value, 'yyyy-MM'))
const monthLabel = computed(() => format(currentMonthDate.value, 'MMMM yyyy'))

const projectGroups = ref<SimpleProjectGroup[]>([])
const saving = ref(false)
const generating = ref(false)

const hasValidTasks = computed(() =>
  projectGroups.value.some(g => g.tasks.some(t => t.title.trim().length > 0))
)

const knownProjects = computed(() => {
  const set = new Set<string>()
  for (const p of profileStore.profile?.projects || []) {
    if (p.trim()) set.add(p.trim())
  }
  for (const g of projectGroups.value) {
    if (g.project.trim()) set.add(g.project.trim())
  }
  return Array.from(set).sort()
})

function loadGroupsFromTasks(tasks: Task[]) {
  if (!tasks || tasks.length === 0) {
    projectGroups.value = [
      {
        project: '',
        tasks: [{ id: 'new-' + Date.now(), title: '', description: '' }],
      },
    ]
    return
  }

  const map: Record<string, SimpleTaskItem[]> = {}
  const order: string[] = []
  for (const t of tasks) {
    const key = t.project?.trim() || ''
    if (!map[key]) {
      map[key] = []
      order.push(key)
    }
    map[key].push({
      id: t.id,
      title: t.title,
      description: t.detailedDescription || t.activitiesPerformed || t.shortDescription || '',
    })
  }

  projectGroups.value = order.map(proj => ({
    project: proj,
    tasks: map[proj],
  }))
}

watch(currentMonthStr, async (m) => {
  await tasksStore.fetchTasks(m)
  loadGroupsFromTasks(tasksStore.tasks)
}, { immediate: true })

function prevMonth() {
  currentMonthDate.value = subMonths(currentMonthDate.value, 1)
}

function nextMonth() {
  currentMonthDate.value = addMonths(currentMonthDate.value, 1)
}

function addProjectGroup() {
  projectGroups.value.push({
    project: '',
    tasks: [
      {
        id: 'new-' + Date.now() + '-' + projectGroups.value.length,
        title: '',
        description: '',
      },
    ],
  })
}

function removeProjectGroup(pIdx: number) {
  if (projectGroups.value.length === 1) {
    projectGroups.value[0] = {
      project: '',
      tasks: [{ id: 'new-' + Date.now(), title: '', description: '' }],
    }
    return
  }
  projectGroups.value.splice(pIdx, 1)
}

function addTaskToProject(pIdx: number) {
  projectGroups.value[pIdx]?.tasks.push({
    id: 'new-' + Date.now() + '-' + Math.random().toString(36).slice(2, 6),
    title: '',
    description: '',
  })
}

function removeTaskFromProject(pIdx: number, tIdx: number) {
  const grp = projectGroups.value[pIdx]
  if (!grp) return
  if (grp.tasks.length === 1) {
    grp.tasks[0] = { id: 'new-' + Date.now(), title: '', description: '' }
    return
  }
  grp.tasks.splice(tIdx, 1)
}

async function saveAllMonthTasks(silent = false): Promise<boolean> {
  const toSave: Task[] = []
  const nowISO = new Date().toISOString()
  const defaultDate = `${currentMonthStr.value}-01`

  for (const grp of projectGroups.value) {
    const projName = grp.project.trim()
    for (const item of grp.tasks) {
      if (!item.title.trim()) continue
      const desc = item.description.trim()
      const shortDesc = desc.length > 140 ? desc.slice(0, 137) + '...' : desc
      const existing = tasksStore.tasks.find(t => t.id === item.id)

      toSave.push({
        id: existing ? existing.id : item.id,
        date: existing?.date || defaultDate,
        project: projName,
        title: item.title.trim(),
        shortDescription: shortDesc,
        detailedDescription: desc,
        issueIdentified: existing?.issueIdentified || '',
        activitiesPerformed: desc,
        solutionImplemented: existing?.solutionImplemented || '',
        outcome: existing?.outcome || '',
        status: existing?.status || 'completed',
        tags: existing?.tags || [],
        source: 'fullmonth',
        createdAt: existing?.createdAt || nowISO,
        updatedAt: nowISO,
      })
    }
  }

  if (toSave.length === 0) {
    uiStore.notify('Please enter at least one task title before saving', 'error')
    return false
  }

  saving.value = true
  try {
    await tasksStore.bulkImport(toSave)
    loadGroupsFromTasks(tasksStore.tasks)
    if (!silent) {
      uiStore.notify(`Saved ${toSave.length} task(s) for ${monthLabel.value}!`, 'success')
    }
    return true
  } catch (err: any) {
    uiStore.notify('Failed to save tasks: ' + (err?.message || 'Unknown error'), 'error')
    return false
  } finally {
    saving.value = false
  }
}

async function generateMonthlyReport() {
  if (!hasValidTasks.value) {
    uiStore.notify('Please enter at least one task title first', 'error')
    return
  }
  generating.value = true
  try {
    const saved = await saveAllMonthTasks(true)
    if (!saved) return
    await reportsApi.generate(currentMonthStr.value, [])
    uiStore.notify('Report generated! Opening preview...', 'success')
    router.push({ path: '/generate-report', query: { month: currentMonthStr.value, step: '5' } })
  } catch (err: any) {
    uiStore.notify('Failed to generate report: ' + (err?.response?.data?.error || err?.message || 'Unknown error'), 'error')
  } finally {
    generating.value = false
  }
}

onMounted(async () => {
  if (!profileStore.profile) {
    await profileStore.fetchProfile()
  }
})
</script>

<template>
  <div class="p-6 max-w-5xl mx-auto space-y-6">
    <!-- Header -->
    <div class="bg-white p-5 rounded-2xl shadow-sm border border-gray-100 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          <Layers class="w-6 h-6 text-primary-600" /> Monthly Tasks
        </h1>
        <p class="text-sm text-gray-500 mt-1">
          Add all tasks for the whole month with optional Project Name, Title, and Description. Add multiple tasks inside each project.
        </p>
      </div>

      <div class="flex items-center gap-3">
        <div class="flex items-center gap-1 bg-gray-50 border border-gray-200 rounded-xl px-2 py-1">
          <button @click="prevMonth" class="p-1 hover:bg-gray-200 rounded-lg"><ChevronLeft class="w-4 h-4" /></button>
          <span class="text-xs font-semibold px-3 min-w-[115px] text-center">{{ monthLabel }}</span>
          <button @click="nextMonth" class="p-1 hover:bg-gray-200 rounded-lg"><ChevronRight class="w-4 h-4" /></button>
        </div>

        <button @click="saveAllMonthTasks(false)" :disabled="saving || generating" class="btn-primary text-xs flex items-center gap-1.5 shadow-sm">
          <Save class="w-4 h-4" /> {{ saving ? 'Saving...' : 'Save Month Tasks' }}
        </button>
      </div>
    </div>

    <datalist id="monthly-projects-list">
      <option v-for="p in knownProjects" :key="p" :value="p" />
    </datalist>

    <!-- Project Cards -->
    <div class="space-y-5">
      <div
        v-for="(grp, pIdx) in projectGroups"
        :key="pIdx"
        class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden"
      >
        <!-- Project Header -->
        <div class="px-5 py-4 bg-gray-50 border-b border-gray-100 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div class="flex-1">
            <label class="block text-[11px] font-semibold uppercase tracking-wider text-gray-500 mb-1">
              Project Name <span class="font-normal text-gray-400">(Optional)</span>
            </label>
            <input
              v-model="grp.project"
              list="monthly-projects-list"
              type="text"
              class="input-field bg-white font-semibold"
              placeholder="Leave blank or enter project name (e.g. ERA 3.0)"
            />
          </div>

          <div class="flex items-end gap-2 pt-2 sm:pt-5">
            <button
              @click="addTaskToProject(pIdx)"
              class="btn-secondary text-xs flex items-center gap-1.5 border-primary-200 text-primary-700 hover:bg-primary-50"
            >
              <Plus class="w-3.5 h-3.5" /> Add Task to Project
            </button>
            <button
              @click="removeProjectGroup(pIdx)"
              class="p-2 rounded-lg border border-red-200 text-red-500 hover:bg-red-50"
              title="Remove Project"
            >
              <Trash2 class="w-4 h-4" />
            </button>
          </div>
        </div>

        <!-- Tasks inside this Project -->
        <div class="divide-y divide-gray-100">
          <div
            v-for="(task, tIdx) in grp.tasks"
            :key="task.id"
            class="p-5 space-y-3"
          >
            <div class="flex items-center justify-between gap-3">
              <span class="text-xs font-bold text-primary-600 bg-primary-50 px-2.5 py-0.5 rounded-full">
                Task {{ tIdx + 1 }}
              </span>
              <button
                @click="removeTaskFromProject(pIdx, tIdx)"
                class="text-xs text-red-500 hover:text-red-700 flex items-center gap-1"
              >
                <Trash2 class="w-3.5 h-3.5" /> Remove
              </button>
            </div>

            <div>
              <label class="label">Title <span class="text-red-500">*</span></label>
              <input
                v-model="task.title"
                type="text"
                class="input-field"
                placeholder="Enter task title"
              />
            </div>

            <div>
              <label class="label">Description</label>
              <textarea
                v-model="task.description"
                rows="3"
                class="input-field resize-y"
                placeholder="Enter task description or bullet points..."
              />
            </div>
          </div>
        </div>

        <!-- Bottom quick add button inside same project -->
        <div class="px-5 py-3 bg-gray-50/60 border-t border-gray-100 flex justify-between items-center">
          <span class="text-xs text-gray-400">{{ grp.tasks.length }} task(s) in this project</span>
          <button
            @click="addTaskToProject(pIdx)"
            class="text-xs font-semibold text-primary-600 hover:text-primary-800 flex items-center gap-1"
          >
            <Plus class="w-3.5 h-3.5" /> Add More Task in Same Project
          </button>
        </div>
      </div>
    </div>

    <!-- Bottom Actions: + Add Another Project, Save & Generate Report -->
    <div class="flex flex-wrap items-center justify-between gap-4 pt-2">
      <button
        @click="addProjectGroup"
        class="btn-secondary text-xs flex items-center gap-1.5"
      >
        <FolderPlus class="w-4 h-4" /> Add Another Project
      </button>

      <div class="flex items-center gap-3 flex-wrap">
        <button
          @click="saveAllMonthTasks(false)"
          :disabled="saving || generating"
          class="btn-secondary text-sm px-5 py-2.5 flex items-center gap-2"
        >
          <Check class="w-4 h-4" /> {{ saving ? 'Saving...' : `Save Tasks` }}
        </button>

        <button
          @click="generateMonthlyReport"
          :disabled="!hasValidTasks || generating || saving"
          class="btn-primary text-sm px-6 py-2.5 flex items-center gap-2 shadow-sm disabled:opacity-50"
        >
          <Sparkles class="w-4 h-4" /> {{ generating ? 'Generating Report...' : 'Generate Report' }}
        </button>
      </div>
    </div>
  </div>
</template>
