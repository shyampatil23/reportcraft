<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useProfileStore } from '@/stores/profile'
import { useUIStore } from '@/stores/ui'
import { reportsApi, templatesApi, type PendingMonthReminder } from '@/api'
import {
  LayoutDashboard, CalendarDays, FileText, ListTodo, Sparkles,
  History, Layout, User, Settings, ChevronRight, X, Check,
  AlertCircle, Info, Menu, Plus
} from 'lucide-vue-next'

const router = useRouter()
const route = useRoute()
const profileStore = useProfileStore()
const uiStore = useUIStore()

// Profile setup form
const setupForm = ref({
  employeeName: '',
  employeeCode: '',
  appraiserName: '',
  department: '',
  team: '',
  projectsText: '',
})
const setupLoading = ref(false)
const setupError = ref('')

const navItems = [
  { label: 'Dashboard', icon: LayoutDashboard, to: '/dashboard' },
  { label: 'Daily Tasks', icon: CalendarDays, to: '/daily-tasks' },
  { label: 'Monthly Tasks', icon: ListTodo, to: '/monthly-tasks' },
  { label: 'Full Month Entry', icon: FileText, to: '/full-month' },
  { label: 'My Tasks', icon: ListTodo, to: '/my-tasks' },
  { label: 'Generate Report', icon: Sparkles, to: '/generate-report' },
  { label: 'Report History', icon: History, to: '/report-history' },
  { label: 'Templates', icon: Layout, to: '/templates' },
]
const bottomNavItems = [
  { label: 'Profile', icon: User, to: '/profile' },
  { label: 'Settings', icon: Settings, to: '/settings' },
]

function isActive(path: string) {
  return route.path === path || route.path.startsWith(path + '/')
}

function navigate(to: string) {
  router.push(to)
}

async function saveSetup() {
  if (!setupForm.value.employeeName.trim()) {
    setupError.value = 'Employee Name is required'
    return
  }
  setupLoading.value = true
  setupError.value = ''
  const projects = setupForm.value.projectsText
    .split(',')
    .map(p => p.trim())
    .filter(p => p.length > 0)
  const ok = await profileStore.saveProfile({
    employeeName: setupForm.value.employeeName,
    employeeCode: setupForm.value.employeeCode,
    appraiserName: setupForm.value.appraiserName,
    department: setupForm.value.department,
    team: setupForm.value.team,
    projects,
  })
  setupLoading.value = false
  if (!ok) setupError.value = 'Failed to save profile. Please try again.'
}

const initials = computed(() => {
  const name = profileStore.profile?.employeeName || ''
  return name.split(' ').map(w => w[0]).join('').toUpperCase().slice(0, 2) || 'U'
})

const notifClass = computed(() => {
  const t = uiStore.notification?.type
  if (t === 'error') return 'bg-red-600'
  if (t === 'info') return 'bg-blue-600'
  return 'bg-green-600'
})

function confirmOk() {
  uiStore.confirmDialog.onConfirm?.()
  uiStore.closeConfirm()
}

const pendingReminder = ref<PendingMonthReminder | null>(null)
const reminderGenerating = ref(false)
let lastCheckedDate = ''
let midnightTimer: ReturnType<typeof setInterval> | null = null

async function checkReminders(force = false) {
  if (profileStore.isFirstLaunch) return
  const today = new Date().toISOString().slice(0, 10)
  if (!force && today === lastCheckedDate) return
  lastCheckedDate = today
  try {
    const evalRes = await reportsApi.reminders()
    if (evalRes.pendingReminder) {
      pendingReminder.value = evalRes.pendingReminder
    } else if (evalRes.draftNotice) {
      uiStore.notify(
        `Draft reminder: Your ${evalRes.draftNotice.monthLabel} report is saved as a draft (${evalRes.draftNotice.status}). Mark it completed when ready.`,
        'info'
      )
    }
  } catch {
    // ignore reminder errors
  }
}

async function handleReminderGenerate() {
  if (!pendingReminder.value) return
  const rem = pendingReminder.value
  reminderGenerating.value = true
  try {
    let defaultTmplId = 'era'
    let defaultMode = 'match_template'
    try {
      const tmpls = await templatesApi.list()
      const def = tmpls.find(t => t.isDefault) || tmpls[0]
      if (def) {
        defaultTmplId = def.id
        defaultMode = def.formattingMode || 'match_template'
      }
    } catch {
      // fallback
    }
    await reportsApi.generate(rem.month, [], defaultTmplId, defaultMode, false)
    pendingReminder.value = null
    uiStore.notify(`Generated ${rem.monthLabel} Performance Report!`, 'success')
    router.push({ path: '/generate-report', query: { month: rem.month, step: '5' } })
  } catch (e: any) {
    uiStore.notify(`Failed to generate report: ${e?.message || 'Unknown error'}`, 'error')
  } finally {
    reminderGenerating.value = false
  }
}

function handleReminderReviewTasks() {
  pendingReminder.value = null
  router.push('/my-tasks')
}

async function handleReminderLater() {
  if (!pendingReminder.value) return
  const [y, m] = pendingReminder.value.month.split('-')
  try {
    await reportsApi.remindLater(y, m)
  } catch {
    // ignore
  }
  pendingReminder.value = null
}

onMounted(async () => {
  await profileStore.fetchProfile()
  await checkReminders(true)
  midnightTimer = setInterval(() => checkReminders(false), 60000)
})

onBeforeUnmount(() => {
  if (midnightTimer) clearInterval(midnightTimer)
})
</script>

<template>
  <div class="h-screen flex overflow-hidden bg-gray-50">

    <!-- First Launch Profile Setup -->
    <Transition name="fade">
      <div v-if="profileStore.isFirstLaunch" class="fixed inset-0 z-50 flex items-center justify-center bg-gradient-to-br from-indigo-950 via-violet-900 to-purple-900">
        <div class="w-full max-w-lg bg-white rounded-3xl shadow-2xl overflow-hidden animate-slide-up">
          <!-- Header -->
          <div class="bg-gradient-to-r from-primary-600 to-violet-600 p-8 text-white text-center">
            <div class="w-16 h-16 bg-white/20 rounded-2xl flex items-center justify-center mx-auto mb-4">
              <Sparkles class="w-8 h-8 text-white" />
            </div>
            <h1 class="text-2xl font-bold">Welcome to Performance Report Generator</h1>
            <p class="text-primary-100 mt-2 text-sm">Set up your profile to get started</p>
          </div>

          <!-- Form -->
          <div class="p-8 space-y-4">
            <div v-if="setupError" class="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg text-sm flex items-center gap-2">
              <AlertCircle class="w-4 h-4 flex-shrink-0" /> {{ setupError }}
            </div>

            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="label">Employee Name <span class="text-red-500">*</span></label>
                <input v-model="setupForm.employeeName" type="text" class="input-field" placeholder="John Doe" />
              </div>
              <div>
                <label class="label">Employee Code</label>
                <input v-model="setupForm.employeeCode" type="text" class="input-field" placeholder="EMP001" />
              </div>
            </div>

            <div>
              <label class="label">Appraiser Name</label>
              <input v-model="setupForm.appraiserName" type="text" class="input-field" placeholder="Manager's name" />
            </div>

            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="label">Department</label>
                <input v-model="setupForm.department" type="text" class="input-field" placeholder="Engineering" />
              </div>
              <div>
                <label class="label">Team</label>
                <input v-model="setupForm.team" type="text" class="input-field" placeholder="Platform" />
              </div>
            </div>

            <div>
              <label class="label">Projects (comma-separated)</label>
              <input v-model="setupForm.projectsText" type="text" class="input-field" placeholder="EMACloud, ERA 3.0, ProjectX" />
              <p class="text-xs text-gray-400 mt-1">You can add or change projects later in your profile settings</p>
            </div>

            <button @click="saveSetup" :disabled="setupLoading" class="btn-primary w-full py-3 text-base font-semibold mt-2 flex items-center justify-center gap-2">
              <span v-if="setupLoading">Setting up...</span>
              <span v-else class="flex items-center gap-2"><Check class="w-5 h-5" /> Save Profile & Get Started</span>
            </button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Main App Layout -->
    <template v-if="!profileStore.isFirstLaunch">
      <!-- Sidebar -->
      <aside class="w-64 flex-shrink-0 bg-indigo-950 flex flex-col h-full overflow-y-auto">
        <!-- Logo -->
        <div class="p-5 border-b border-indigo-800">
          <div class="flex items-center gap-3">
            <div class="w-9 h-9 bg-gradient-to-br from-primary-500 to-violet-500 rounded-xl flex items-center justify-center flex-shrink-0">
              <Sparkles class="w-5 h-5 text-white" />
            </div>
            <div>
              <div class="text-white font-bold text-sm leading-tight">Performance</div>
              <div class="text-indigo-300 text-xs">Report Generator</div>
            </div>
          </div>
        </div>

        <!-- Nav -->
        <nav class="flex-1 p-3 space-y-1 mt-2">
          <button
            v-for="item in navItems"
            :key="item.to"
            @click="navigate(item.to)"
            class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm font-medium transition-all duration-150"
            :class="isActive(item.to)
              ? 'bg-primary-600 text-white shadow-lg'
              : 'text-indigo-300 hover:bg-indigo-800 hover:text-white'"
          >
            <component :is="item.icon" class="w-4.5 h-4.5 flex-shrink-0" style="width:18px;height:18px" />
            {{ item.label }}
          </button>

          <div class="border-t border-indigo-800 my-3" />

          <button
            v-for="item in bottomNavItems"
            :key="item.to"
            @click="navigate(item.to)"
            class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm font-medium transition-all duration-150"
            :class="isActive(item.to)
              ? 'bg-primary-600 text-white shadow-lg'
              : 'text-indigo-300 hover:bg-indigo-800 hover:text-white'"
          >
            <component :is="item.icon" class="w-4.5 h-4.5 flex-shrink-0" style="width:18px;height:18px" />
            {{ item.label }}
          </button>
        </nav>

        <!-- User pill at bottom -->
        <div class="p-3 border-t border-indigo-800">
          <button @click="navigate('/profile')" class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl hover:bg-indigo-800 transition-all">
            <div class="w-8 h-8 rounded-full bg-gradient-to-br from-primary-500 to-violet-500 flex items-center justify-center text-white text-xs font-bold flex-shrink-0">
              {{ initials }}
            </div>
            <div class="flex-1 min-w-0 text-left">
              <div class="text-white text-sm font-medium truncate">{{ profileStore.profile?.employeeName || 'Employee' }}</div>
              <div class="text-indigo-400 text-xs truncate">{{ profileStore.profile?.department || '' }}</div>
            </div>
          </button>
        </div>
      </aside>

      <!-- Main Content -->
      <main class="flex-1 overflow-y-auto">
        <RouterView />
      </main>
    </template>

    <!-- Global Notification Toast -->
    <Transition name="slide-up">
      <div
        v-if="uiStore.notification"
        class="fixed bottom-6 right-6 z-50 flex items-center gap-3 px-5 py-3.5 rounded-xl shadow-2xl text-white text-sm font-medium max-w-sm"
        :class="notifClass"
      >
        <Check v-if="uiStore.notification.type === 'success'" class="w-5 h-5 flex-shrink-0" />
        <AlertCircle v-else-if="uiStore.notification.type === 'error'" class="w-5 h-5 flex-shrink-0" />
        <Info v-else class="w-5 h-5 flex-shrink-0" />
        <span>{{ uiStore.notification.message }}</span>
        <button @click="uiStore.notification = null" class="ml-2 opacity-70 hover:opacity-100">
          <X class="w-4 h-4" />
        </button>
      </div>
    </Transition>

    <!-- Confirm Dialog -->
    <Transition name="fade">
      <div v-if="uiStore.confirmDialog.visible" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
        <div class="bg-white rounded-2xl shadow-2xl p-8 max-w-sm w-full mx-4 animate-slide-up">
          <h3 class="text-lg font-bold text-gray-900 mb-2">{{ uiStore.confirmDialog.title }}</h3>
          <p class="text-gray-600 text-sm mb-6">{{ uiStore.confirmDialog.message }}</p>
          <div class="flex gap-3 justify-end">
            <button @click="uiStore.closeConfirm()" class="btn-secondary">Cancel</button>
            <button @click="confirmOk()" class="btn-danger">Confirm</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Monthly Performance Report Reminder Popup -->
    <Transition name="fade">
      <div v-if="pendingReminder" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
        <div class="bg-white rounded-2xl shadow-2xl max-w-md w-full overflow-hidden animate-slide-up">
          <div class="bg-gradient-to-r from-primary-600 to-violet-600 px-6 py-4 text-white flex items-center gap-3">
            <Sparkles class="w-5 h-5" />
            <h3 class="text-sm font-bold tracking-wide uppercase">Monthly Performance Report Reminder</h3>
          </div>
          <div class="p-6 space-y-4">
            <p class="text-sm text-gray-700 leading-relaxed">
              Your <strong>{{ pendingReminder.monthName }}</strong> Performance Report is pending
              ({{ pendingReminder.taskCount }} saved task{{ pendingReminder.taskCount === 1 ? '' : 's' }}).
              Would you like to generate it now?
            </p>
            <div class="flex flex-wrap gap-2 justify-end pt-2">
              <button @click="handleReminderLater" class="btn-ghost text-xs">
                Remind Me Later
              </button>
              <button @click="handleReminderReviewTasks" class="btn-secondary text-xs">
                Review Tasks
              </button>
              <button @click="handleReminderGenerate" :disabled="reminderGenerating" class="btn-primary text-xs flex items-center gap-1.5">
                <Sparkles class="w-3.5 h-3.5" />
                <span>{{ reminderGenerating ? 'Generating...' : 'Generate Report' }}</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>
