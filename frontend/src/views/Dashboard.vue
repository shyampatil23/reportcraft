<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useProfileStore } from '@/stores/profile'
import { useTasksStore } from '@/stores/tasks'
import { reportsApi, type MonthlyReportStatus, type ReminderEvaluation } from '@/api'
import { format } from 'date-fns'
import {
  TrendingUp, CheckCircle2, Calendar, FolderOpen,
  Plus, Sparkles, ChevronRight, Clock, FileText, Zap
} from 'lucide-vue-next'

const router = useRouter()
const profileStore = useProfileStore()
const tasksStore = useTasksStore()

const currentMonth = ref(format(new Date(), 'yyyy-MM'))
const monthLabel = computed(() => {
  const [y, m] = currentMonth.value.split('-')
  return new Date(+y, +m - 1).toLocaleDateString('en-US', { month: 'long', year: 'numeric' })
})
const todayLabel = computed(() => format(new Date(), 'EEEE, MMMM d, yyyy'))

const greeting = computed(() => {
  const h = new Date().getHours()
  if (h < 12) return 'Good morning'
  if (h < 17) return 'Good afternoon'
  return 'Good evening'
})

const stats = computed(() => [
  {
    label: 'Total Tasks',
    value: tasksStore.stats?.totalTasks ?? 0,
    icon: TrendingUp,
    color: 'from-primary-500 to-primary-600',
    bg: 'bg-primary-50',
    text: 'text-primary-600',
    border: 'border-l-primary-500',
  },
  {
    label: 'Days with Entries',
    value: tasksStore.stats?.daysWithEntries ?? 0,
    icon: Calendar,
    color: 'from-blue-500 to-blue-600',
    bg: 'bg-blue-50',
    text: 'text-blue-600',
    border: 'border-l-blue-500',
  },
  {
    label: 'Completed Tasks',
    value: tasksStore.stats?.completedTasks ?? 0,
    icon: CheckCircle2,
    color: 'from-green-500 to-green-600',
    bg: 'bg-green-50',
    text: 'text-green-600',
    border: 'border-l-green-500',
  },
  {
    label: 'Active Projects',
    value: tasksStore.stats?.projects?.length ?? 0,
    icon: FolderOpen,
    color: 'from-violet-500 to-violet-600',
    bg: 'bg-violet-50',
    text: 'text-violet-600',
    border: 'border-l-violet-500',
  },
])

const statusClass: Record<string, string> = {
  completed: 'badge-status-completed',
  'in-progress': 'badge-status-in-progress',
  pending: 'badge-status-pending',
}

const monthStatus = ref<MonthlyReportStatus | null>(null)
const reminderEval = ref<ReminderEvaluation | null>(null)

const reportStatusBadge = computed(() => {
  const s = monthStatus.value?.status || 'IN_PROGRESS'
  if (s === 'COMPLETED') return { label: 'COMPLETED', cls: 'bg-green-100 text-green-800 border-green-200' }
  if (s === 'EDITED_DRAFT') return { label: 'EDITED_DRAFT', cls: 'bg-indigo-100 text-indigo-800 border-indigo-200' }
  if (s === 'GENERATED_DRAFT') return { label: 'GENERATED_DRAFT', cls: 'bg-blue-100 text-blue-800 border-blue-200' }
  return { label: 'IN_PROGRESS', cls: 'bg-amber-100 text-amber-800 border-amber-200' }
})

async function fetchMonthStatus() {
  const [y, m] = currentMonth.value.split('-')
  try {
    const res = await reportsApi.status(y, m)
    monthStatus.value = res.statusInfo
  } catch {
    monthStatus.value = null
  }
}

function onMonthChange(e: Event) {
  currentMonth.value = (e.target as HTMLInputElement).value
  tasksStore.fetchTasks(currentMonth.value)
  tasksStore.fetchStats(currentMonth.value)
  fetchMonthStatus()
}

onMounted(async () => {
  await Promise.all([
    tasksStore.fetchTasks(currentMonth.value),
    tasksStore.fetchStats(currentMonth.value),
    fetchMonthStatus(),
    reportsApi.reminders().then(r => { reminderEval.value = r }).catch(() => {}),
  ])
})
</script>

<template>
  <div class="min-h-full bg-gray-50 p-8 animate-fade-in">
    <!-- Header -->
    <div class="flex flex-wrap items-center justify-between gap-4 mb-8">
      <div>
        <h1 class="text-2xl font-bold text-gray-900">
          {{ greeting }}, {{ profileStore.profile?.employeeName?.split(' ')[0] || 'there' }}! 👋
        </h1>
        <p class="text-sm text-gray-500 mt-0.5">{{ todayLabel }}</p>
      </div>
      <div class="flex items-center gap-3">
        <input
          type="month"
          :value="currentMonth"
          @change="onMonthChange"
          class="input-field w-auto px-3 py-2 text-sm"
        />
        <button @click="router.push('/daily-tasks')" class="btn-primary flex items-center gap-2">
          <Plus class="w-4 h-4" /> Add Task
        </button>
      </div>
    </div>

    <!-- Last Day of Month Prominent Banner -->
    <div
      v-if="reminderEval?.isLastDayOfMonth"
      class="mb-6 rounded-2xl bg-gradient-to-r from-primary-600 to-violet-600 p-5 text-white shadow-lg flex flex-col sm:flex-row sm:items-center justify-between gap-4"
    >
      <div>
        <div class="text-xs font-bold uppercase tracking-wider text-primary-100">Month-End Ready</div>
        <h2 class="text-lg font-bold mt-0.5">Your monthly performance report is ready to generate.</h2>
      </div>
      <div class="flex items-center gap-2.5 shrink-0">
        <button @click="router.push('/my-tasks')" class="px-4 py-2 rounded-xl bg-white/15 hover:bg-white/25 text-white text-xs font-semibold">
          Review Tasks
        </button>
        <button @click="router.push({ path: '/generate-report', query: { month: reminderEval.currentMonth } })" class="px-4 py-2 rounded-xl bg-white text-primary-700 hover:bg-primary-50 text-xs font-bold shadow-sm">
          Generate {{ reminderEval.currentMonthName }} Report
        </button>
      </div>
    </div>

    <!-- Monthly Performance Report Status Card -->
    <div class="card mb-6 flex flex-col lg:flex-row lg:items-center justify-between gap-4 border border-primary-100">
      <div class="space-y-1">
        <div class="text-xs font-bold uppercase tracking-wider text-primary-600">Monthly Performance Report</div>
        <div class="flex items-center gap-3 flex-wrap">
          <span class="text-lg font-bold text-gray-900">{{ monthLabel }}</span>
          <span :class="['text-xs font-bold px-2.5 py-0.5 rounded-full border', reportStatusBadge.cls]">
            {{ reportStatusBadge.label }}
          </span>
          <span class="text-xs text-gray-500">Tasks Entered: <strong>{{ tasksStore.stats?.totalTasks ?? 0 }}</strong></span>
        </div>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <button @click="router.push('/daily-tasks')" class="btn-secondary text-xs py-2 px-3">Add Daily Task</button>
        <button @click="router.push('/full-month')" class="btn-secondary text-xs py-2 px-3">Full Month Entry</button>
        <button @click="router.push({ path: '/generate-report', query: { month: currentMonth } })" class="btn-primary text-xs py-2 px-3 flex items-center gap-1.5">
          <Sparkles class="w-3.5 h-3.5" /> Generate Report
        </button>
        <button @click="router.push('/report-history')" class="btn-secondary text-xs py-2 px-3">Report History</button>
      </div>
    </div>

    <!-- Stats Grid -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-5 mb-8">
      <div
        v-for="stat in stats"
        :key="stat.label"
        class="card border-l-4 hover:shadow-md transition-shadow duration-200"
        :class="stat.border"
      >
        <div class="flex items-center justify-between mb-3">
          <div class="w-10 h-10 rounded-xl flex items-center justify-center" :class="stat.bg">
            <component :is="stat.icon" class="w-5 h-5" :class="stat.text" />
          </div>
          <span class="text-3xl font-bold text-gray-900">{{ stat.value }}</span>
        </div>
        <p class="text-sm font-medium text-gray-500">{{ stat.label }}</p>
        <p class="text-xs text-gray-400 mt-0.5">{{ monthLabel }}</p>
      </div>
    </div>

    <!-- Two-column section -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <!-- Recent Activity -->
      <div class="card lg:col-span-2">
        <div class="flex items-center justify-between mb-5">
          <h2 class="text-base font-semibold text-gray-900 flex items-center gap-2">
            <Clock class="w-4 h-4 text-primary-500" /> Recent Task Activity
          </h2>
          <button @click="router.push('/my-tasks')" class="text-xs text-primary-600 hover:text-primary-700 font-medium flex items-center gap-1">
            View All <ChevronRight class="w-3.5 h-3.5" />
          </button>
        </div>

        <div v-if="tasksStore.stats?.recentTasks?.length" class="space-y-3">
          <div
            v-for="task in tasksStore.stats.recentTasks"
            :key="task.id"
            class="flex items-start gap-3 p-3 rounded-xl hover:bg-gray-50 transition-colors cursor-pointer border border-transparent hover:border-gray-100"
            @click="router.push('/daily-tasks')"
          >
            <div class="w-8 h-8 rounded-lg bg-primary-50 flex items-center justify-center flex-shrink-0 mt-0.5">
              <FileText class="w-4 h-4 text-primary-500" />
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="text-xs font-medium px-2 py-0.5 rounded-full bg-primary-100 text-primary-700">{{ task.project || 'No project' }}</span>
                <span :class="statusClass[task.status]">{{ task.status }}</span>
              </div>
              <p class="text-sm font-medium text-gray-800 mt-1 truncate">{{ task.title }}</p>
              <p class="text-xs text-gray-400 mt-0.5">{{ task.date }}</p>
            </div>
          </div>
        </div>

        <!-- Empty state -->
        <div v-else class="text-center py-12 text-gray-400">
          <div class="text-5xl mb-3">📋</div>
          <p class="font-medium text-gray-500">No tasks this month yet</p>
          <p class="text-sm mt-1">Start by adding daily tasks or pasting your full month's work</p>
          <button @click="router.push('/daily-tasks')" class="btn-primary mt-4 text-sm">
            Add First Task
          </button>
        </div>
      </div>

      <!-- Quick Actions -->
      <div class="card">
        <h2 class="text-base font-semibold text-gray-900 mb-5 flex items-center gap-2">
          <Zap class="w-4 h-4 text-amber-500" /> Quick Actions
        </h2>
        <div class="space-y-3">
          <button
            @click="router.push('/daily-tasks')"
            class="w-full flex items-center gap-3 p-3.5 rounded-xl border border-primary-200 hover:bg-primary-50 hover:border-primary-300 transition-all text-left group"
          >
            <div class="w-9 h-9 bg-primary-100 rounded-lg flex items-center justify-center group-hover:bg-primary-200 transition-colors">
              <Plus class="w-5 h-5 text-primary-600" />
            </div>
            <div>
              <p class="text-sm font-semibold text-gray-900">Add Task Today</p>
              <p class="text-xs text-gray-500">Log today's work</p>
            </div>
          </button>

          <button
            @click="router.push('/full-month')"
            class="w-full flex items-center gap-3 p-3.5 rounded-xl border border-gray-200 hover:bg-gray-50 hover:border-gray-300 transition-all text-left group"
          >
            <div class="w-9 h-9 bg-gray-100 rounded-lg flex items-center justify-center group-hover:bg-gray-200 transition-colors">
              <FileText class="w-5 h-5 text-gray-600" />
            </div>
            <div>
              <p class="text-sm font-semibold text-gray-900">Full Month Entry</p>
              <p class="text-xs text-gray-500">Paste all month's work at once</p>
            </div>
          </button>

          <button
            @click="router.push('/generate-report')"
            class="w-full flex items-center gap-3 p-3.5 rounded-xl bg-gradient-to-r from-primary-600 to-violet-600 hover:from-primary-700 hover:to-violet-700 transition-all text-left shadow-md hover:shadow-lg"
          >
            <div class="w-9 h-9 bg-white/20 rounded-lg flex items-center justify-center">
              <Sparkles class="w-5 h-5 text-white" />
            </div>
            <div>
              <p class="text-sm font-semibold text-white">Generate Report</p>
              <p class="text-xs text-primary-200">Create monthly PDF/Word report</p>
            </div>
          </button>

          <button
            @click="router.push('/full-month')"
            class="w-full flex items-center gap-3 p-3.5 rounded-xl border border-violet-200 hover:bg-violet-50 transition-all text-left group"
          >
            <div class="w-9 h-9 bg-violet-100 rounded-lg flex items-center justify-center group-hover:bg-violet-200 transition-colors">
              <Sparkles class="w-5 h-5 text-violet-600" />
            </div>
            <div>
              <p class="text-sm font-semibold text-gray-900">AI Writing Assistant</p>
              <p class="text-xs text-gray-500">Improve task descriptions with AI</p>
            </div>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
