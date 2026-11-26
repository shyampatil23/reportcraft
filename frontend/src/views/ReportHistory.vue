<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  History, Download, FileText, Eye, Calendar,
  Sparkles, RefreshCw, X, Pencil, CheckCircle2
} from 'lucide-vue-next'
import { format } from 'date-fns'
import { useUIStore } from '@/stores/ui'
import { reportsApi, editorApi, templatesApi, type MonthlyReportStatus } from '@/api'

interface HistoryEntry {
  month: string
  monthLabel: string
  status: string
  statusInfo: MonthlyReportStatus
  taskCount: number
  hasReport: boolean
  report?: any
  hasEdits: boolean
}

const router = useRouter()
const uiStore = useUIStore()

const items = ref<HistoryEntry[]>([])
const loading = ref(false)
const filterYear = ref('')
const filterMonth = ref('')
const filterStatus = ref('')

const previewModalOpen = ref(false)
const selectedEntry = ref<HistoryEntry | null>(null)
const previewHtmlUrl = ref('')

const yearsList = computed(() => {
  const s = new Set(items.value.map(i => i.month.split('-')[0]).filter(Boolean))
  return Array.from(s).sort().reverse()
})

const filteredItems = computed(() => {
  return items.value.filter(i => {
    const [y, m] = i.month.split('-')
    if (filterYear.value && y !== filterYear.value) return false
    if (filterMonth.value && m !== filterMonth.value) return false
    if (filterStatus.value && i.status !== filterStatus.value) return false
    return true
  })
})

function statusBadgeClass(status: string) {
  if (status === 'COMPLETED') return 'bg-green-100 text-green-800 border-green-200'
  if (status === 'EDITED_DRAFT') return 'bg-indigo-100 text-indigo-800 border-indigo-200'
  if (status === 'GENERATED_DRAFT') return 'bg-blue-100 text-blue-800 border-blue-200'
  return 'bg-amber-100 text-amber-800 border-amber-200'
}

function openPreview(entry: HistoryEntry) {
  selectedEntry.value = entry
  const [y, m] = entry.month.split('-')
  previewHtmlUrl.value = reportsApi.previewUrl(y, m) + '?t=' + Date.now()
  previewModalOpen.value = true
}

function editReport(entry: HistoryEntry) {
  router.push({ path: '/generate-report', query: { month: entry.month, step: '5' } })
}

function downloadPdf(entry: HistoryEntry) {
  const [y, m] = entry.month.split('-')
  window.open(reportsApi.exportPdfUrl(y, m), '_blank')
}

function downloadDocx(entry: HistoryEntry) {
  const [y, m] = entry.month.split('-')
  window.location.href = reportsApi.exportDocxUrl(y, m)
}

async function regenerateEntry(entry: HistoryEntry) {
  const [y, m] = entry.month.split('-')
  let forceOverwrite = false
  try {
    const st = await editorApi.status(y, m)
    if (st.hasSavedEdits || st.hasDraft) {
      const ok = window.confirm(
        `Manual edits exist for ${entry.monthLabel}. Regenerating will replace manual edits (a backup will be saved).\n\nClick OK to regenerate, or Cancel to keep existing edits.`
      )
      if (!ok) return
      forceOverwrite = true
    }
  } catch {
    // ignore
  }
  try {
    let tmplId = entry.report?.templateId || 'era'
    let mode = entry.report?.formattingMode || 'match_template'
    try {
      const tmpls = await templatesApi.list()
      const def = tmpls.find(t => t.isDefault) || tmpls[0]
      if (!entry.report?.templateId && def) {
        tmplId = def.id
        mode = def.formattingMode || 'match_template'
      }
    } catch {
      // ignore
    }
    await reportsApi.generate(entry.month, [], tmplId, mode, forceOverwrite)
    uiStore.notify(`Regenerated ${entry.monthLabel} report!`, 'success')
    await refreshList()
  } catch (e: any) {
    uiStore.notify(`Regeneration failed: ${e?.message || 'Unknown error'}`, 'error')
  }
}

async function refreshList() {
  loading.value = true
  try {
    items.value = await reportsApi.history()
  } catch (e: any) {
    uiStore.notify('Failed to load report history', 'error')
  } finally {
    loading.value = false
  }
}

onMounted(refreshList)
</script>

<template>
  <div class="p-6 max-w-7xl mx-auto space-y-6">
    <!-- Header -->
    <div class="bg-white p-5 rounded-2xl shadow-sm border border-gray-100 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          <History class="w-6 h-6 text-primary-600" /> Monthly Report History
        </h1>
        <p class="text-sm text-gray-500 mt-1">
          View, edit, download, or regenerate monthly performance reports across all months.
        </p>
      </div>

      <div class="flex items-center gap-3 flex-wrap">
        <select v-model="filterYear" class="input-field w-auto text-xs py-2">
          <option value="">All Years</option>
          <option v-for="y in yearsList" :key="y" :value="y">{{ y }}</option>
        </select>
        <select v-model="filterMonth" class="input-field w-auto text-xs py-2">
          <option value="">All Months</option>
          <option v-for="m in 12" :key="m" :value="String(m).padStart(2, '0')">
            {{ new Date(2026, m - 1, 1).toLocaleDateString('en-US', { month: 'long' }) }}
          </option>
        </select>
        <select v-model="filterStatus" class="input-field w-auto text-xs py-2">
          <option value="">All Statuses</option>
          <option value="COMPLETED">COMPLETED</option>
          <option value="EDITED_DRAFT">EDITED_DRAFT</option>
          <option value="GENERATED_DRAFT">GENERATED_DRAFT</option>
          <option value="IN_PROGRESS">IN_PROGRESS</option>
        </select>
        <button @click="refreshList" class="btn-secondary text-xs flex items-center gap-1.5">
          <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': loading }" /> Refresh
        </button>
        <router-link to="/generate-report" class="btn-primary text-xs flex items-center gap-1.5 shadow-sm">
          <Sparkles class="w-3.5 h-3.5" /> Generate New Report
        </router-link>
      </div>
    </div>

    <!-- Reports Grid -->
    <div v-if="loading" class="text-center py-16 text-gray-400">
      <div class="animate-spin w-8 h-8 border-4 border-primary-500 border-t-transparent rounded-full mx-auto mb-3"></div>
      Loading report history...
    </div>

    <div v-else-if="filteredItems.length === 0" class="bg-white rounded-2xl p-12 text-center shadow-sm border border-gray-100">
      <FileText class="w-12 h-12 text-gray-300 mx-auto mb-3" />
      <h3 class="text-base font-bold text-gray-800">No Matching Reports Found</h3>
      <p class="text-xs text-gray-500 max-w-sm mx-auto mt-1 mb-4">
        Adjust your filters or generate a monthly report to see it listed here.
      </p>
      <router-link to="/generate-report" class="btn-primary text-xs inline-flex items-center gap-1.5">
        <Sparkles class="w-3.5 h-3.5" /> Create Report
      </router-link>
    </div>

    <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
      <div
        v-for="entry in filteredItems"
        :key="entry.month"
        class="bg-white rounded-2xl p-5 shadow-sm border border-gray-100 hover:shadow-md transition flex flex-col justify-between space-y-4"
      >
        <div class="space-y-3">
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-bold text-primary-700 bg-primary-50 px-2.5 py-1 rounded-lg border border-primary-100 flex items-center gap-1.5">
              <Calendar class="w-3.5 h-3.5" /> {{ entry.monthLabel }}
            </span>
            <span :class="['text-[11px] font-bold px-2.5 py-0.5 rounded-full border', statusBadgeClass(entry.status)]">
              {{ entry.status }}
            </span>
          </div>

          <div>
            <h3 class="text-base font-bold text-gray-900">
              {{ entry.report?.coverPage?.title || 'Monthly Performance Report' }}
            </h3>
            <p class="text-xs text-gray-500 mt-1 line-clamp-1">
              Projects: {{ entry.report?.coverPage?.projects?.join(', ') || 'Pending generation' }}
            </p>
          </div>

          <div class="text-[11px] text-gray-400 border-t border-gray-50 pt-2 flex items-center justify-between">
            <span>{{ entry.taskCount }} task(s)</span>
            <span v-if="entry.statusInfo?.completedAt" class="text-green-600 font-medium flex items-center gap-1">
              <CheckCircle2 class="w-3 h-3" /> {{ format(new Date(entry.statusInfo.completedAt), 'MMM d, yyyy') }}
            </span>
            <span v-else-if="entry.statusInfo?.generatedAt">
              Gen: {{ format(new Date(entry.statusInfo.generatedAt), 'MMM d, yyyy') }}
            </span>
          </div>
        </div>

        <!-- Actions -->
        <div class="flex flex-wrap gap-1.5 pt-2 border-t border-gray-100">
          <button
            v-if="entry.hasReport"
            @click="openPreview(entry)"
            class="btn-secondary text-xs py-1.5 px-2.5 flex items-center gap-1"
          >
            <Eye class="w-3.5 h-3.5" /> View Report
          </button>
          <button
            v-if="entry.hasReport"
            @click="editReport(entry)"
            class="btn-secondary text-xs py-1.5 px-2.5 flex items-center gap-1"
          >
            <Pencil class="w-3.5 h-3.5" /> Edit Report
          </button>
          <button
            v-if="entry.hasReport"
            @click="downloadPdf(entry)"
            class="btn-secondary text-xs py-1.5 px-2.5 flex items-center gap-1 text-red-600"
          >
            <Download class="w-3.5 h-3.5" /> PDF
          </button>
          <button
            v-if="entry.hasReport"
            @click="downloadDocx(entry)"
            class="btn-secondary text-xs py-1.5 px-2.5 flex items-center gap-1 text-blue-600"
          >
            <FileText class="w-3.5 h-3.5" /> DOCX
          </button>
          <button
            @click="regenerateEntry(entry)"
            class="btn-secondary text-xs py-1.5 px-2.5 flex items-center gap-1 ml-auto"
          >
            <RefreshCw class="w-3.5 h-3.5" /> Regenerate
          </button>
        </div>
      </div>
    </div>

    <!-- Preview Modal -->
    <div v-if="previewModalOpen && selectedEntry" class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4">
      <div class="bg-white rounded-2xl shadow-2xl w-full max-w-5xl h-[90vh] flex flex-col overflow-hidden animate-slide-up">
        <div class="px-6 py-4 border-b border-gray-100 flex items-center justify-between bg-gray-50">
          <div>
            <h3 class="text-base font-bold text-gray-900">
              {{ selectedEntry.monthLabel }} Performance Report
            </h3>
            <p class="text-xs text-gray-500">Status: {{ selectedEntry.status }}</p>
          </div>

          <div class="flex items-center gap-2">
            <button @click="editReport(selectedEntry)" class="btn-secondary text-xs py-1.5 px-3 flex items-center gap-1.5">
              <Pencil class="w-3.5 h-3.5" /> Edit Report
            </button>
            <button @click="downloadPdf(selectedEntry)" class="btn-primary text-xs py-1.5 px-3 flex items-center gap-1.5">
              <Download class="w-3.5 h-3.5" /> Download PDF
            </button>
            <button @click="downloadDocx(selectedEntry)" class="btn-secondary text-xs py-1.5 px-3 flex items-center gap-1.5">
              <FileText class="w-3.5 h-3.5" /> Download Word
            </button>
            <button @click="previewModalOpen = false" class="p-1.5 text-gray-400 hover:text-gray-700 rounded-lg">
              <X class="w-5 h-5" />
            </button>
          </div>
        </div>

        <div class="flex-1 bg-gray-100 p-4 overflow-hidden">
          <iframe
            :src="previewHtmlUrl"
            class="w-full h-full bg-white rounded-xl shadow-inner border border-gray-200"
          ></iframe>
        </div>
      </div>
    </div>
  </div>
</template>
