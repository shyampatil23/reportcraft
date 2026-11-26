<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import {
  ChevronLeft, ChevronRight, Plus, Pencil, Trash2, Copy, Sparkles,
  X, Check, Calendar as CalendarIcon, Clock, AlertCircle, FileText, Tag
} from 'lucide-vue-next'
import {
  format, getDaysInMonth, startOfMonth, getDay, isToday, parseISO,
  addMonths, subMonths, setYear, setMonth
} from 'date-fns'
import { useTasksStore } from '@/stores/tasks'
import { useProfileStore } from '@/stores/profile'
import { useUIStore } from '@/stores/ui'
import { useRouter } from 'vue-router'
import { aiApi, reportsApi, type ReminderEvaluation } from '@/api'
import type { Task } from '@/types'

const router = useRouter()
const tasksStore = useTasksStore()
const profileStore = useProfileStore()
const uiStore = useUIStore()
const reminderEval = ref<ReminderEvaluation | null>(null)

// Current viewing month and selected date
const currentDate = ref(new Date())
const selectedDate = ref(format(new Date(), 'yyyy-MM-dd'))
const showForm = ref(false)
const isEditing = ref(false)
const aiLoading = ref(false)
const activeAiField = ref<string | null>(null)

// Task Form
const defaultTaskForm = (): Partial<Task> => ({
  date: selectedDate.value,
  project: profileStore.profile?.projects?.[0] || '',
  title: '',
  shortDescription: '',
  detailedDescription: '',
  issueIdentified: '',
  activitiesPerformed: '',
  solutionImplemented: '',
  outcome: '',
  status: 'completed',
  tags: [],
})

const taskForm = ref<Partial<Task>>(defaultTaskForm())
const tagsInput = ref('')

const currentMonthStr = computed(() => format(currentDate.value, 'yyyy-MM'))
const monthYearTitle = computed(() => format(currentDate.value, 'MMMM yyyy'))
const viewingMonthName = computed(() => format(currentDate.value, 'MMMM'))
const isSelectedDateLastDayOfMonth = computed(() => {
  if (!selectedDate.value) return false
  const [y, m, d] = selectedDate.value.split('-').map(Number)
  if (!y || !m || !d) return false
  return d === getDaysInMonth(new Date(y, m - 1, 1))
})

// Watch month change and fetch tasks
watch(currentMonthStr, async (newMonth) => {
  await tasksStore.fetchTasks(newMonth)
}, { immediate: true })

// Calendar Calculation
const daysOfWeek = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

interface CalendarDay {
  dateStr: string
  dayNum: number
  isCurrentMonth: boolean
  isToday: boolean
  isSelected: boolean
  tasks: Task[]
}

const calendarDays = computed<CalendarDay[]>(() => {
  const days: CalendarDay[] = []
  const daysInMonth = getDaysInMonth(currentDate.value)
  const firstDayOfMonth = getDay(startOfMonth(currentDate.value))

  // Blank padding for days before start of month
  for (let i = 0; i < firstDayOfMonth; i++) {
    days.push({
      dateStr: '',
      dayNum: 0,
      isCurrentMonth: false,
      isToday: false,
      isSelected: false,
      tasks: [],
    })
  }

  // Days in month
  const year = currentDate.value.getFullYear()
  const month = currentDate.value.getMonth() + 1

  for (let d = 1; d <= daysInMonth; d++) {
    const dStr = `${year}-${String(month).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    const tasksOnDay = tasksStore.tasksByDate[dStr] || []
    days.push({
      dateStr: dStr,
      dayNum: d,
      isCurrentMonth: true,
      isToday: isToday(new Date(year, month - 1, d)),
      isSelected: selectedDate.value === dStr,
      tasks: tasksOnDay,
    })
  }

  return days
})

const selectedDateTasks = computed(() => {
  return tasksStore.tasksByDate[selectedDate.value] || []
})

const formattedSelectedDate = computed(() => {
  try {
    return format(parseISO(selectedDate.value), 'EEEE, MMMM d, yyyy')
  } catch {
    return selectedDate.value
  }
})

function prevMonth() {
  currentDate.value = subMonths(currentDate.value, 1)
}

function nextMonth() {
  currentDate.value = addMonths(currentDate.value, 1)
}

function selectDay(day: CalendarDay) {
  if (!day.isCurrentMonth) return
  selectedDate.value = day.dateStr
  taskForm.value.date = day.dateStr
}

function openNewTaskForm() {
  isEditing.value = false
  taskForm.value = defaultTaskForm()
  tagsInput.value = ''
  showForm.value = true
}

function openEditTaskForm(task: Task) {
  isEditing.value = true
  taskForm.value = { ...task }
  tagsInput.value = (task.tags || []).join(', ')
  showForm.value = true
}

async function saveTask() {
  if (!taskForm.value.title?.trim()) {
    uiStore.notify('Task title is required', 'error')
    return
  }

  taskForm.value.tags = tagsInput.value
    .split(',')
    .map(t => t.trim())
    .filter(t => t.length > 0)

  if (isEditing.value && taskForm.value.id) {
    await tasksStore.updateTask(taskForm.value.id, taskForm.value)
    uiStore.notify('Task updated successfully')
  } else {
    await tasksStore.createTask(taskForm.value)
    uiStore.notify('Task added successfully')
  }

  showForm.value = false
  tasksStore.fetchStats(currentMonthStr.value)
}

async function deleteTask(id: string) {
  uiStore.confirm('Delete Task', 'Are you sure you want to delete this task?', async () => {
    await tasksStore.deleteTask(id)
    uiStore.notify('Task deleted')
    tasksStore.fetchStats(currentMonthStr.value)
  })
}

async function duplicateTask(task: Task) {
  const clone: Partial<Task> = {
    ...task,
    id: undefined,
    title: `${task.title} (Copy)`,
  }
  await tasksStore.createTask(clone)
  uiStore.notify('Task duplicated')
  tasksStore.fetchStats(currentMonthStr.value)
}

// AI Assist on input fields
async function improveField(fieldKey: keyof Task, mode: 'improve' | 'expand' | 'activities' | 'issue' | 'solution' | 'outcome') {
  const val = (taskForm.value[fieldKey] as string) || ''
  if (!val.trim()) {
    uiStore.notify('Enter some initial text before using AI improvement', 'info')
    return
  }

  aiLoading.value = true
  activeAiField.value = fieldKey
  try {
    const res = await aiApi.improve({
      text: val,
      mode,
      detail: 'standard',
      context: `Task Title: ${taskForm.value.title || ''}, Project: ${taskForm.value.project || ''}`,
    })
    if (res && res.improved) {
      ;(taskForm.value as any)[fieldKey] = res.improved
      uiStore.notify('Improved with AI!')
    }
  } catch (err: any) {
    const msg = err.response?.data?.error || err.message || 'AI is unavailable offline or not configured.'
    uiStore.notify(msg, 'error')
  } finally {
    aiLoading.value = false
    activeAiField.value = null
  }
}

onMounted(() => {
  tasksStore.fetchTasks(currentMonthStr.value)
  reportsApi.reminders().then(r => { reminderEval.value = r }).catch(() => {})
})
</script>

<template>
  <div class="p-6 max-w-7xl mx-auto space-y-6">
    <!-- Top Action & Navigation Header -->
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 bg-white p-5 rounded-2xl shadow-sm border border-gray-100">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          <CalendarIcon class="w-6 h-6 text-primary-600" /> Daily Task Entry
        </h1>
        <p class="text-sm text-gray-500 mt-1">
          Select any date on the calendar to record or modify your day's contributions.
        </p>
      </div>

      <div class="flex items-center gap-3">
        <div class="flex items-center bg-gray-50 rounded-xl border border-gray-200 p-1">
          <button @click="prevMonth" class="p-2 hover:bg-white rounded-lg text-gray-600 transition shadow-xs">
            <ChevronLeft class="w-5 h-5" />
          </button>
          <span class="px-4 font-semibold text-gray-800 text-sm select-none">{{ monthYearTitle }}</span>
          <button @click="nextMonth" class="p-2 hover:bg-white rounded-lg text-gray-600 transition shadow-xs">
            <ChevronRight class="w-5 h-5" />
          </button>
        </div>

        <button @click="openNewTaskForm" class="btn-primary flex items-center gap-2 text-sm shadow-sm">
          <Plus class="w-4 h-4" /> Add Task for Selected Date
        </button>
      </div>
    </div>

    <!-- Last Day of Month Banner -->
    <div
      v-if="reminderEval?.isLastDayOfMonth"
      class="rounded-2xl bg-gradient-to-r from-primary-600 to-violet-600 p-4 text-white shadow-md flex flex-col sm:flex-row sm:items-center justify-between gap-3"
    >
      <div>
        <span class="text-xs font-bold uppercase tracking-wider text-primary-100">Last Day of Month</span>
        <p class="text-sm font-bold">Your monthly performance report is ready to generate.</p>
      </div>
      <div class="flex items-center gap-2 shrink-0">
        <button @click="router.push('/my-tasks')" class="px-3 py-1.5 rounded-lg bg-white/15 hover:bg-white/25 text-white text-xs font-semibold">
          Review Tasks
        </button>
        <button @click="router.push({ path: '/generate-report', query: { month: reminderEval.currentMonth } })" class="px-3 py-1.5 rounded-lg bg-white text-primary-700 hover:bg-primary-50 text-xs font-bold">
          Generate {{ reminderEval.currentMonthName }} Report
        </button>
      </div>
    </div>

    <!-- Main Grid: Calendar on Left, Selected Day Tasks on Right -->
    <div class="grid grid-cols-1 lg:grid-cols-12 gap-6">
      <!-- Calendar View (7 cols on LG) -->
      <div class="lg:col-span-7 bg-white rounded-2xl p-5 shadow-sm border border-gray-100">
        <div class="grid grid-cols-7 gap-2 mb-2">
          <div v-for="d in daysOfWeek" :key="d" class="text-center font-semibold text-xs text-gray-400 uppercase py-2">
            {{ d }}
          </div>
        </div>

        <div class="grid grid-cols-7 gap-2">
          <div
            v-for="(day, idx) in calendarDays"
            :key="idx"
            @click="selectDay(day)"
            :class="[
              'min-h-[85px] p-2 rounded-xl transition border text-left flex flex-col justify-between cursor-pointer select-none',
              !day.isCurrentMonth ? 'bg-transparent border-transparent cursor-default' : 'hover:border-primary-400 bg-gray-50/50',
              day.isSelected ? 'border-primary-600 ring-2 ring-primary-500/20 bg-primary-50/40' : 'border-gray-100',
              day.isToday && !day.isSelected ? 'border-violet-400 bg-violet-50/30' : ''
            ]"
          >
            <template v-if="day.isCurrentMonth">
              <div class="flex items-center justify-between">
                <span :class="[
                  'text-xs font-bold w-6 h-6 rounded-full flex items-center justify-center',
                  day.isSelected ? 'bg-primary-600 text-white' : day.isToday ? 'bg-violet-600 text-white' : 'text-gray-700'
                ]">
                  {{ day.dayNum }}
                </span>
                <span v-if="day.tasks.length > 0" class="text-[10px] font-semibold text-primary-700 bg-primary-100/80 px-1.5 py-0.5 rounded-full">
                  {{ day.tasks.length }}
                </span>
              </div>

              <!-- Dot indicators or mini list -->
              <div class="mt-1 space-y-1 overflow-hidden">
                <div
                  v-for="t in day.tasks.slice(0, 2)"
                  :key="t.id"
                  class="text-[10px] truncate px-1.5 py-0.5 rounded bg-white border border-gray-200 text-gray-700 font-medium shadow-2xs"
                >
                  {{ t.title }}
                </div>
                <div v-if="day.tasks.length > 2" class="text-[9px] text-gray-400 pl-1 font-medium">
                  +{{ day.tasks.length - 2 }} more
                </div>
              </div>
            </template>
          </div>
        </div>
      </div>

      <!-- Right Panel: Tasks of Selected Day (5 cols on LG) -->
      <div class="lg:col-span-5 space-y-4">
        <div class="bg-white rounded-2xl p-5 shadow-sm border border-gray-100 flex flex-col h-full">
          <div class="flex items-center justify-between pb-4 border-b border-gray-100">
            <div>
              <span class="text-xs font-semibold uppercase tracking-wider text-primary-600">Selected Day</span>
              <h3 class="text-base font-bold text-gray-900">{{ formattedSelectedDate }}</h3>
            </div>
            <button @click="openNewTaskForm" class="btn-primary text-xs py-1.5 px-3 flex items-center gap-1.5">
              <Plus class="w-3.5 h-3.5" /> Add Task
            </button>
          </div>

          <!-- Month-End Generate Report Prompt when Selected Day is Last Day of Month -->
          <div
            v-if="isSelectedDateLastDayOfMonth"
            class="mt-4 rounded-xl bg-gradient-to-r from-primary-600 to-violet-600 p-3.5 text-white shadow-sm flex flex-col gap-2"
          >
            <div>
              <div class="text-[10px] font-bold uppercase tracking-wider text-primary-100">Final Day of {{ viewingMonthName }}</div>
              <p class="text-xs font-semibold">Your monthly performance report is ready to generate.</p>
            </div>
            <div class="flex items-center gap-2">
              <button @click="router.push('/my-tasks')" class="px-2.5 py-1 rounded-lg bg-white/15 hover:bg-white/25 text-white text-[11px] font-semibold">
                Review Tasks
              </button>
              <button @click="router.push({ path: '/generate-report', query: { month: currentMonthStr } })" class="px-2.5 py-1 rounded-lg bg-white text-primary-700 hover:bg-primary-50 text-[11px] font-bold">
                Generate {{ viewingMonthName }} Report
              </button>
            </div>
          </div>

          <!-- Task List for day -->
          <div class="py-4 flex-1 space-y-3 overflow-y-auto max-h-[600px]">
            <div v-if="selectedDateTasks.length === 0" class="text-center py-12 text-gray-400">
              <FileText class="w-10 h-10 mx-auto text-gray-300 mb-2 stroke-[1.5]" />
              <p class="text-sm font-medium text-gray-600">No tasks logged on this day</p>
              <p class="text-xs text-gray-400 mt-1">Click "Add Task" to enter what you worked on.</p>
            </div>

            <div
              v-for="task in selectedDateTasks"
              :key="task.id"
              class="border border-gray-200/80 rounded-xl p-4 hover:shadow-sm transition bg-white space-y-2 group"
            >
              <div class="flex items-start justify-between gap-2">
                <div>
                  <span class="inline-block text-[11px] font-semibold text-primary-700 bg-primary-50 px-2 py-0.5 rounded-md border border-primary-100 mb-1">
                    {{ task.project || 'Unassigned Project' }}
                  </span>
                  <h4 class="text-sm font-bold text-gray-900">{{ task.title }}</h4>
                </div>

                <div class="flex items-center gap-1 opacity-90 sm:opacity-0 group-hover:opacity-100 transition">
                  <button @click="openEditTaskForm(task)" title="Edit" class="p-1 text-gray-500 hover:text-primary-600 rounded hover:bg-gray-100">
                    <Pencil class="w-3.5 h-3.5" />
                  </button>
                  <button @click="duplicateTask(task)" title="Duplicate" class="p-1 text-gray-500 hover:text-blue-600 rounded hover:bg-gray-100">
                    <Copy class="w-3.5 h-3.5" />
                  </button>
                  <button @click="deleteTask(task.id)" title="Delete" class="p-1 text-gray-500 hover:text-red-600 rounded hover:bg-gray-100">
                    <Trash2 class="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>

              <p v-if="task.shortDescription" class="text-xs text-gray-600 line-clamp-2">
                {{ task.shortDescription }}
              </p>

              <div class="pt-2 flex items-center justify-between text-[11px] border-t border-gray-100">
                <span :class="[
                  'px-2 py-0.5 rounded-full font-medium',
                  task.status === 'completed' ? 'bg-green-100 text-green-700' :
                  task.status === 'in-progress' ? 'bg-blue-100 text-blue-700' : 'bg-amber-100 text-amber-700'
                ]">
                  {{ task.status }}
                </span>

                <div v-if="task.tags && task.tags.length > 0" class="flex gap-1 flex-wrap">
                  <span v-for="tag in task.tags" :key="tag" class="text-gray-500 bg-gray-100 px-1.5 py-0.5 rounded text-[10px]">
                    #{{ tag }}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Modal Form: Add / Edit Task -->
    <div v-if="showForm" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-xs p-4 overflow-y-auto">
      <div class="bg-white rounded-2xl shadow-xl w-full max-w-2xl overflow-hidden animate-slide-up my-8">
        <div class="px-6 py-4 border-b border-gray-100 flex items-center justify-between bg-gray-50">
          <div>
            <h3 class="text-base font-bold text-gray-900">{{ isEditing ? 'Edit Task' : 'Add New Daily Task' }}</h3>
            <p class="text-xs text-gray-500">Date: {{ taskForm.date }}</p>
          </div>
          <button @click="showForm = false" class="p-1.5 text-gray-400 hover:text-gray-700 rounded-lg">
            <X class="w-5 h-5" />
          </button>
        </div>

        <div class="p-6 space-y-4 max-h-[75vh] overflow-y-auto">
          <!-- Project & Status -->
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="label">Project <span class="text-red-500">*</span></label>
              <input
                v-model="taskForm.project"
                type="text"
                class="input-field"
                placeholder="Select or enter project name"
                list="project-options"
              />
              <datalist id="project-options">
                <option v-for="p in profileStore.profile?.projects" :key="p" :value="p" />
              </datalist>
            </div>

            <div>
              <label class="label">Status</label>
              <select v-model="taskForm.status" class="input-field">
                <option value="completed">Completed</option>
                <option value="in-progress">In Progress</option>
                <option value="pending">Pending</option>
              </select>
            </div>
          </div>

          <!-- Task Title -->
          <div>
            <label class="label">Task Title <span class="text-red-500">*</span></label>
            <input
              v-model="taskForm.title"
              type="text"
              class="input-field"
              placeholder="e.g. Translated Video Integration API"
            />
          </div>

          <!-- Short Description -->
          <div>
            <div class="flex items-center justify-between mb-1">
              <label class="label mb-0">Short Description</label>
              <button
                @click="improveField('shortDescription', 'improve')"
                type="button"
                class="text-xs text-primary-600 hover:text-primary-700 font-semibold flex items-center gap-1"
                :disabled="aiLoading"
              >
                <Sparkles class="w-3.5 h-3.5" /> Improve with AI
              </button>
            </div>
            <textarea
              v-model="taskForm.shortDescription"
              rows="2"
              class="textarea-field"
              placeholder="Brief summary of the work done..."
            ></textarea>
          </div>

          <!-- Detailed Description -->
          <div>
            <div class="flex items-center justify-between mb-1">
              <label class="label mb-0">Detailed Description / Work Log</label>
              <button
                @click="improveField('detailedDescription', 'expand')"
                type="button"
                class="text-xs text-primary-600 hover:text-primary-700 font-semibold flex items-center gap-1"
                :disabled="aiLoading"
              >
                <Sparkles class="w-3.5 h-3.5" /> Expand with AI
              </button>
            </div>
            <textarea
              v-model="taskForm.detailedDescription"
              rows="3"
              class="textarea-field"
              placeholder="Full details, technical changes made, modules touched..."
            ></textarea>
          </div>

          <!-- Structured Report Fields Accordion -->
          <div class="border border-purple-100 bg-purple-50/30 rounded-xl p-4 space-y-3">
            <h4 class="text-xs font-bold text-purple-900 uppercase tracking-wider flex items-center gap-1.5">
              <FileText class="w-3.5 h-3.5 text-purple-600" />
              Report Structure Fields (Optional - auto-generated if left blank)
            </h4>

            <div>
              <div class="flex items-center justify-between mb-1">
                <span class="text-xs font-semibold text-gray-700">Issue Identified:</span>
                <button @click="improveField('issueIdentified', 'issue')" type="button" class="text-[11px] text-purple-700 hover:underline flex items-center gap-1">
                  <Sparkles class="w-3 h-3" /> AI Draft
                </button>
              </div>
              <textarea v-model="taskForm.issueIdentified" rows="2" class="textarea-field bg-white text-xs" placeholder="What challenge or requirement necessitated this task?"></textarea>
            </div>

            <div>
              <div class="flex items-center justify-between mb-1">
                <span class="text-xs font-semibold text-gray-700">Activities Performed:</span>
                <button @click="improveField('activitiesPerformed', 'activities')" type="button" class="text-[11px] text-purple-700 hover:underline flex items-center gap-1">
                  <Sparkles class="w-3 h-3" /> AI Draft
                </button>
              </div>
              <textarea v-model="taskForm.activitiesPerformed" rows="2" class="textarea-field bg-white text-xs" placeholder="Step by step steps taken..."></textarea>
            </div>

            <div>
              <div class="flex items-center justify-between mb-1">
                <span class="text-xs font-semibold text-gray-700">Solution Implemented:</span>
                <button @click="improveField('solutionImplemented', 'solution')" type="button" class="text-[11px] text-purple-700 hover:underline flex items-center gap-1">
                  <Sparkles class="w-3 h-3" /> AI Draft
                </button>
              </div>
              <textarea v-model="taskForm.solutionImplemented" rows="2" class="textarea-field bg-white text-xs" placeholder="Architectural or technical fix applied..."></textarea>
            </div>

            <div>
              <div class="flex items-center justify-between mb-1">
                <span class="text-xs font-semibold text-gray-700">Outcome:</span>
                <button @click="improveField('outcome', 'outcome')" type="button" class="text-[11px] text-purple-700 hover:underline flex items-center gap-1">
                  <Sparkles class="w-3 h-3" /> AI Draft
                </button>
              </div>
              <textarea v-model="taskForm.outcome" rows="2" class="textarea-field bg-white text-xs" placeholder="Measurable result, performance improvement, user impact..."></textarea>
            </div>
          </div>

          <!-- Tags -->
          <div>
            <label class="label">Tags (comma-separated)</label>
            <input
              v-model="tagsInput"
              type="text"
              class="input-field"
              placeholder="e.g. backend, API, bugfix, refactor"
            />
          </div>
        </div>

        <div class="px-6 py-4 border-t border-gray-100 flex items-center justify-end gap-3 bg-gray-50">
          <button @click="showForm = false" class="btn-secondary text-sm">Cancel</button>
          <button @click="saveTask" class="btn-primary text-sm flex items-center gap-2">
            <Check class="w-4 h-4" /> {{ isEditing ? 'Update Task' : 'Save Task' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
