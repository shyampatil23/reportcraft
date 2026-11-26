<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import {
  Settings, Database, FolderOpen, Download, UploadCloud,
  Sparkles, Key, GitBranch, ShieldCheck, Check, RefreshCw, AlertCircle
} from 'lucide-vue-next'
import { settingsApi, backupApi, aiApi } from '@/api'
import { useUIStore } from '@/stores/ui'
import type { Settings as SettingsModel, BackupInfo } from '@/types'

const uiStore = useUIStore()

const loading = ref(false)
const isTesting = ref(false)
const dataPath = ref('')
const backups = ref<BackupInfo[]>([])

const settingsForm = ref<SettingsModel>({
  aiEnabled: false,
  aiProvider: 'gemini',
  aiModel: 'gemini-3.5-flash',
  gitlabEnabled: false,
  gitlabUrl: 'https://gitlab.com',
  autoBackup: true,
  backupFrequency: 'daily',
  dataPath: '',
})

// AI Creds
const aiApiKey = ref('')
const aiBaseUrl = ref('')

// GitLab Creds
const gitlabToken = ref('')

// Auto-detect provider if user pastes Google or OpenAI key
watch(aiApiKey, (newVal) => {
  const k = newVal.trim()
  if (k.startsWith('AIzaSy') || k.startsWith('AQ.')) {
    if (settingsForm.value.aiProvider !== 'gemini') {
      settingsForm.value.aiProvider = 'gemini'
      settingsForm.value.aiModel = 'gemini-3.5-flash'
      uiStore.notify('Detected Google Gemini key - switched provider to Gemini', 'info')
    }
  } else if (k.startsWith('sk-')) {
    if (settingsForm.value.aiProvider !== 'openai') {
      settingsForm.value.aiProvider = 'openai'
      settingsForm.value.aiModel = 'gpt-4o-mini'
      uiStore.notify('Detected OpenAI key - switched provider to OpenAI', 'info')
    }
  }
})

async function loadData() {
  loading.value = true
  try {
    const [s, df, bl, creds] = await Promise.all([
      settingsApi.get(),
      backupApi.dataFolder(),
      backupApi.list(),
      aiApi.getCreds().catch(() => null),
    ])
    if (s) settingsForm.value = s
    if (df) dataPath.value = df.path
    if (bl) backups.value = bl
    if (creds) {
      if (creds.provider) settingsForm.value.aiProvider = creds.provider
      if (creds.model) settingsForm.value.aiModel = creds.model
      if (creds.baseUrl) aiBaseUrl.value = creds.baseUrl
      if (creds.apiKey) {
        aiApiKey.value = creds.apiKey
        settingsForm.value.aiEnabled = true
      }
    }
  } catch (err: any) {
    uiStore.notify('Error loading settings: ' + err.message, 'error')
  } finally {
    loading.value = false
  }
}

async function saveGeneralSettings() {
  try {
    await settingsApi.update(settingsForm.value)
    uiStore.notify('Settings saved successfully!')
  } catch (err: any) {
    uiStore.notify('Failed to save settings: ' + err.message, 'error')
  }
}

async function saveAICreds() {
  try {
    settingsForm.value.aiEnabled = true
    await aiApi.saveCreds(
      settingsForm.value.aiProvider,
      aiApiKey.value.trim(),
      settingsForm.value.aiModel,
      aiBaseUrl.value
    )
    await settingsApi.update(settingsForm.value)
    uiStore.notify('AI credentials saved locally in encrypted storage!')
  } catch (err: any) {
    uiStore.notify('Failed to save AI credentials: ' + err.message, 'error')
  }
}

async function testAIConnection() {
  if (!aiApiKey.value.trim()) {
    uiStore.notify('Please enter an API Key first', 'error')
    return
  }
  isTesting.value = true
  try {
    settingsForm.value.aiEnabled = true
    await aiApi.saveCreds(
      settingsForm.value.aiProvider,
      aiApiKey.value.trim(),
      settingsForm.value.aiModel,
      aiBaseUrl.value
    )
    await settingsApi.update(settingsForm.value)

    const res = await aiApi.test()
    if (res && res.status === 'ok') {
      const timeSec = (res.latencyMs / 1000).toFixed(2)
      uiStore.notify(`✅ Connected successfully in ${timeSec}s! (${res.provider}: ${res.model})`, 'success')
    }
  } catch (err: any) {
    const msg = err.response?.data?.error || err.message
    uiStore.notify('❌ AI Test Failed: ' + msg, 'error')
  } finally {
    isTesting.value = false
  }
}

async function triggerBackup() {
  try {
    const res = await backupApi.create()
    uiStore.notify(`Backup created: ${res.filename} (${res.sizeMB.toFixed(2)} MB)`)
    backups.value = await backupApi.list()
  } catch (err: any) {
    uiStore.notify('Backup failed: ' + err.message, 'error')
  }
}

onMounted(() => {
  loadData()
})
</script>

<template>
  <div class="p-6 max-w-5xl mx-auto space-y-6">
    <!-- Header -->
    <div class="bg-white p-5 rounded-2xl shadow-sm border border-gray-100 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          <Settings class="w-6 h-6 text-primary-600" /> Settings & Data Management
        </h1>
        <p class="text-sm text-gray-500 mt-1">
          Manage your local JSON storage, backups, and optional AI / GitLab API integrations.
        </p>
      </div>

      <button @click="saveGeneralSettings" class="btn-primary text-xs flex items-center gap-2 shadow-sm font-semibold">
        <Check class="w-3.5 h-3.5" /> Save Configuration
      </button>
    </div>

    <!-- Storage & Backup Card -->
    <div class="bg-white rounded-2xl p-6 shadow-sm border border-gray-100 space-y-4">
      <div class="flex items-center justify-between border-b border-gray-100 pb-3">
        <div class="flex items-center gap-2.5">
          <Database class="w-5 h-5 text-primary-600" />
          <h2 class="text-base font-bold text-gray-900">Local JSON Storage & Backups</h2>
        </div>
        <span class="text-xs font-semibold text-emerald-700 bg-emerald-50 px-2.5 py-0.5 rounded-full border border-emerald-200">
          100% Local & Private
        </span>
      </div>

      <div class="space-y-3">
        <div>
          <label class="label">Windows Local Storage Path</label>
          <div class="flex items-center gap-2">
            <input
              type="text"
              readonly
              :value="dataPath || '%LOCALAPPDATA%\\PerformanceReportBuilder\\data'"
              class="input-field bg-gray-50 text-xs font-mono text-gray-600 cursor-text select-all"
            />
          </div>
          <p class="text-[11px] text-gray-400 mt-1">
            All your daily tasks, drafts, templates, and profile are saved strictly on this Windows computer.
          </p>
        </div>

        <div class="pt-2 flex flex-wrap items-center gap-3">
          <button @click="triggerBackup" class="btn-primary text-xs flex items-center gap-1.5 shadow-sm">
            <Download class="w-3.5 h-3.5" /> Backup Now (.zip)
          </button>
        </div>

        <!-- Recent Backups List -->
        <div v-if="backups.length > 0" class="pt-3">
          <h4 class="text-xs font-bold uppercase tracking-wider text-gray-500 mb-2">Available Backups:</h4>
          <div class="divide-y divide-gray-100 border border-gray-100 rounded-xl overflow-hidden text-xs">
            <div v-for="b in backups.slice(0, 5)" :key="b.filename" class="px-4 py-2.5 flex items-center justify-between hover:bg-gray-50">
              <span class="font-mono text-gray-800 font-semibold">{{ b.filename }}</span>
              <span class="text-gray-400">{{ b.sizeMB.toFixed(2) }} MB</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- AI Writing Assistant Configuration Card -->
    <div class="bg-white rounded-2xl p-6 shadow-sm border border-gray-100 space-y-4">
      <div class="flex items-center justify-between border-b border-gray-100 pb-3">
        <div class="flex items-center gap-2.5">
          <Sparkles class="w-5 h-5 text-purple-600" />
          <h2 class="text-base font-bold text-gray-900">AI Writing Assistant (Optional)</h2>
        </div>

        <label class="relative inline-flex items-center cursor-pointer">
          <input type="checkbox" v-model="settingsForm.aiEnabled" class="sr-only peer" />
          <div class="w-11 h-6 bg-gray-200 peer-focus:outline-hidden rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-600"></div>
          <span class="ml-2 text-xs font-semibold text-gray-700">Enable AI Features</span>
        </label>
      </div>

      <p class="text-xs text-gray-500">
        AI is completely optional. If disabled or offline, all report generation, formatting, and offline parsing work seamlessly without internet.
      </p>

      <div v-if="settingsForm.aiEnabled" class="space-y-4 pt-2">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="label">AI Provider</label>
            <select v-model="settingsForm.aiProvider" class="input-field">
              <option value="openai">OpenAI (GPT-4o, GPT-4o-mini)</option>
              <option value="gemini">Google Gemini (Gemini 3.5 Flash)</option>
            </select>
          </div>

          <div>
            <label class="label">Model Name</label>
            <select v-if="settingsForm.aiProvider === 'gemini'" v-model="settingsForm.aiModel" class="input-field">
              <option value="gemini-3.5-flash">gemini-3.5-flash (Fast & Stable - Recommended)</option>
              <option value="gemini-flash-latest">gemini-flash-latest (Latest Auto-Updating Flash)</option>
              <option value="gemini-3.8-flash">gemini-3.8-flash (Gemini 3.8 Flash)</option>
              <option value="gemini-2.5-pro">gemini-2.5-pro (High Quality Pro)</option>
            </select>
            <input
              v-else
              v-model="settingsForm.aiModel"
              type="text"
              class="input-field"
              placeholder="e.g. gpt-4o-mini or gpt-4o"
            />
          </div>
        </div>

        <div>
          <label class="label">API Key</label>
          <input
            v-model="aiApiKey"
            type="password"
            class="input-field font-mono"
            placeholder="sk-... or AIzaSy..."
          />
          <p class="text-[11px] text-gray-400 mt-1">
            Accepts Google Gemini API keys (starts with <code>AIzaSy...</code>) or OpenAI API keys (starts with <code>sk-...</code>). Keys are persisted locally.
          </p>
        </div>

        <div class="flex items-center gap-3 pt-1">
          <button @click="saveAICreds" class="btn-secondary text-xs flex items-center gap-1.5 border-purple-200 text-purple-700 hover:bg-purple-50">
            <Key class="w-3.5 h-3.5" /> Save AI Credentials
          </button>
          <button @click="testAIConnection" :disabled="isTesting" class="btn-primary text-xs flex items-center gap-1.5 shadow-sm">
            <RefreshCw v-if="isTesting" class="w-3.5 h-3.5 animate-spin" />
            <Sparkles v-else class="w-3.5 h-3.5" />
            {{ isTesting ? 'Connecting to Gemini...' : 'Test AI Connection' }}
          </button>
        </div>
      </div>
    </div>

    <!-- Optional GitLab Integration -->
    <div class="bg-white rounded-2xl p-6 shadow-sm border border-gray-100 space-y-4">
      <div class="flex items-center justify-between border-b border-gray-100 pb-3">
        <div class="flex items-center gap-2.5">
          <GitBranch class="w-5 h-5 text-orange-600" />
          <h2 class="text-base font-bold text-gray-900">GitLab Integration (Optional)</h2>
        </div>

        <label class="relative inline-flex items-center cursor-pointer">
          <input type="checkbox" v-model="settingsForm.gitlabEnabled" class="sr-only peer" />
          <div class="w-11 h-6 bg-gray-200 peer-focus:outline-hidden rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-600"></div>
          <span class="ml-2 text-xs font-semibold text-gray-700">Enable GitLab Sync</span>
        </label>
      </div>

      <div v-if="settingsForm.gitlabEnabled" class="space-y-4 pt-2">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="label">GitLab Server URL</label>
            <input
              v-model="settingsForm.gitlabUrl"
              type="text"
              class="input-field"
              placeholder="https://gitlab.com or https://gitlab.yourorg.com"
            />
          </div>

          <div>
            <label class="label">Personal Access Token (read_repository)</label>
            <input
              v-model="gitlabToken"
              type="password"
              class="input-field font-mono"
              placeholder="glpat-..."
            />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
