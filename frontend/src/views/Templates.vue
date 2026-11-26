<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import {
  Layout, Sparkles, Check, UploadCloud, Sliders, Plus,
  Trash2, Eye, Copy, Edit3, GitCompare, ShieldCheck, AlertTriangle,
  Image as ImageIcon, Layers, ZoomIn, ZoomOut, ChevronLeft, ChevronRight
} from 'lucide-vue-next'
import * as pdfjsLib from 'pdfjs-dist'
import { templatesApi, aiApi } from '@/api'
import { useUIStore } from '@/stores/ui'
import type { Template, TemplateFieldMapping } from '@/types'

// Configure PDF.js worker via Vite URL import fallback
pdfjsLib.GlobalWorkerOptions.workerSrc = new URL(
  'pdfjs-dist/build/pdf.worker.min.mjs',
  import.meta.url
).toString()

const uiStore = useUIStore()
const templates = ref<Template[]>([])
const loading = ref(false)

// Import modal state
const showImportModal = ref(false)
const importing = ref(false)
const newTemplateName = ref('')
const newTemplateColor = ref('#1E293B')
const newTemplateAccent = ref('#E2E8F0')
const selectedFile = ref<File | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)

// Rename modal
const renamingTmpl = ref<Template | null>(null)
const renameValue = ref('')

// Wizard modal state
const wizardTmpl = ref<Template | null>(null)
const wizardTab = ref<'analysis' | 'typography' | 'mappings' | 'assets'>('analysis')
const savingWizard = ref(false)
const aiMappingSuggestion = ref('')
const aiSuggesting = ref(false)

// Visual Comparison modal state
const compareTmpl = ref<Template | null>(null)
const compareZoom = ref(0.85)
const overlayMode = ref(false)
const overlayOpacity = ref(0.5)
const pdfCanvasRef = ref<HTMLCanvasElement | null>(null)
const pdfPageNum = ref(1)
const pdfTotalPages = ref(0)
const pdfLoadError = ref('')
let pdfDocProxy: any = null

const fieldTypeOptions = [
  { value: 'cover_page', label: 'Cover Page' },
  { value: 'employee_name', label: 'Employee Name' },
  { value: 'employee_code', label: 'Employee Code' },
  { value: 'appraiser_name', label: 'Appraiser Name' },
  { value: 'month', label: 'Month' },
  { value: 'year', label: 'Year' },
  { value: 'projects', label: 'Project Names' },
  { value: 'toc_area', label: 'Table of Contents Area' },
  { value: 'task_title', label: 'Task Title Style' },
  { value: 'task_description', label: 'Task Description Style' },
  { value: 'repeatable_task', label: 'Repeatable Task Section' },
  { value: 'header', label: 'Header' },
  { value: 'footer', label: 'Footer' },
  { value: 'page_number', label: 'Page Number' },
  { value: 'background_image', label: 'Background Image' },
]

async function loadTemplates() {
  loading.value = true
  try {
    templates.value = await templatesApi.list()
  } catch (err: any) {
    uiStore.notify('Failed to load templates: ' + err.message, 'error')
  } finally {
    loading.value = false
  }
}

function openImport() {
  newTemplateName.value = ''
  newTemplateColor.value = '#1E293B'
  newTemplateAccent.value = '#E2E8F0'
  selectedFile.value = null
  showImportModal.value = true
}

function triggerFileInput() {
  fileInputRef.value?.click()
}

function onFileSelected(event: Event) {
  const target = event.target as HTMLInputElement
  if (target.files && target.files.length > 0) {
    const f = target.files[0]
    selectedFile.value = f
    if (!newTemplateName.value) {
      newTemplateName.value = f.name.replace(/\.[^/.]+$/, '').replace(/[-_]/g, ' ')
    }
  }
}

function onFileDrop(event: DragEvent) {
  if (event.dataTransfer?.files && event.dataTransfer.files.length > 0) {
    const f = event.dataTransfer.files[0]
    selectedFile.value = f
    if (!newTemplateName.value) {
      newTemplateName.value = f.name.replace(/\.[^/.]+$/, '').replace(/[-_]/g, ' ')
    }
  }
}

async function handleImportSubmit() {
  if (!newTemplateName.value.trim()) {
    uiStore.notify('Please provide a name for the template', 'error')
    return
  }
  if (!selectedFile.value) {
    uiStore.notify('Please select a local .docx or .pdf file', 'error')
    return
  }
  importing.value = true
  try {
    const imported = await templatesApi.importFile(
      selectedFile.value,
      newTemplateName.value.trim(),
      newTemplateColor.value,
      newTemplateAccent.value
    )
    templates.value.push(imported)
    showImportModal.value = false
    uiStore.notify(`Template "${imported.name}" analyzed & imported! Opening Mapping Wizard...`, 'success')
    openWizard(imported)
  } catch (err: any) {
    uiStore.notify('Import failed: ' + (err.response?.data?.error || err.message), 'error')
  } finally {
    importing.value = false
  }
}

async function setAsDefault(tmpl: Template) {
  try {
    await templatesApi.setDefault(tmpl.id)
    templates.value.forEach(t => { t.isDefault = t.id === tmpl.id })
    uiStore.notify(`Template "${tmpl.name}" set as active default!`, 'success')
  } catch (err: any) {
    uiStore.notify('Failed to set default: ' + err.message, 'error')
  }
}

async function duplicateTemplate(tmpl: Template) {
  try {
    const clone = await templatesApi.duplicate(tmpl.id)
    templates.value.push(clone)
    uiStore.notify(`Duplicated "${tmpl.name}"`, 'success')
  } catch (err: any) {
    uiStore.notify('Duplicate failed: ' + err.message, 'error')
  }
}

function startRename(tmpl: Template) {
  renamingTmpl.value = tmpl
  renameValue.value = tmpl.name
}

async function confirmRename() {
  if (!renamingTmpl.value || !renameValue.value.trim()) return
  const updated = { ...renamingTmpl.value, name: renameValue.value.trim() }
  try {
    const saved = await templatesApi.update(updated.id, updated)
    const idx = templates.value.findIndex(t => t.id === saved.id)
    if (idx !== -1) templates.value[idx] = saved
    renamingTmpl.value = null
    uiStore.notify('Template renamed', 'success')
  } catch (err: any) {
    uiStore.notify('Rename failed: ' + err.message, 'error')
  }
}

function deleteTemplate(tmpl: Template) {
  if (tmpl.id === 'era') {
    uiStore.notify('Built-in ERA template cannot be deleted', 'error')
    return
  }
  uiStore.confirm(
    'Delete Template',
    `Delete "${tmpl.name}" and its extracted assets?`,
    async () => {
      try {
        await templatesApi.remove(tmpl.id)
        templates.value = templates.value.filter(t => t.id !== tmpl.id)
        uiStore.notify('Template deleted', 'info')
      } catch (err: any) {
        uiStore.notify('Delete failed: ' + err.message, 'error')
      }
    }
  )
}

function openWizard(tmpl: Template) {
  wizardTmpl.value = JSON.parse(JSON.stringify(tmpl))
  if (!wizardTmpl.value!.fieldMappings) wizardTmpl.value!.fieldMappings = []
  aiMappingSuggestion.value = ''
  wizardTab.value = 'analysis'
}

function applyCustomERAPreset() {
  if (!wizardTmpl.value) return
  wizardTmpl.value.formattingMode = 'custom'
  wizardTmpl.value.typography.projectName = { fontFamily: 'Century Gothic', fontSizePt: 23, bold: true, color: wizardTmpl.value.primaryColor || '#262262', align: 'left' }
  wizardTmpl.value.typography.taskTitle = { fontFamily: 'Times New Roman', fontSizePt: 16, bold: true, color: '#111111', align: 'left' }
  wizardTmpl.value.typography.subheading = { fontFamily: 'Times New Roman', fontSizePt: 14, bold: true, color: '#111111', align: 'left' }
  wizardTmpl.value.typography.taskDescription = { fontFamily: 'Times New Roman', fontSizePt: 14, bold: false, color: '#222222', align: 'left' }
  uiStore.notify('Applied 23pt / 16pt / 14pt Custom Formatting rules', 'info')
}

function addMappingRow() {
  if (!wizardTmpl.value) return
  if (!wizardTmpl.value.fieldMappings) wizardTmpl.value.fieldMappings = []
  const newMap: TemplateFieldMapping = {
    id: 'map-' + Date.now(),
    fieldType: 'task_title',
    label: 'New Field Mapping',
    pageNumber: 1,
    x: 10,
    y: 20,
    width: 80,
    height: 10,
    fontFamily: wizardTmpl.value.typography.taskTitle.fontFamily || 'Times New Roman',
    fontSizePt: wizardTmpl.value.typography.taskTitle.fontSizePt || 14,
    bold: false,
    color: '#111111',
    maskExisting: wizardTmpl.value.sourceType === 'pdf',
  }
  wizardTmpl.value.fieldMappings.push(newMap)
}

function removeMappingRow(idx: number) {
  wizardTmpl.value?.fieldMappings?.splice(idx, 1)
}

async function suggestMappingsWithAI() {
  if (!wizardTmpl.value) return
  aiSuggesting.value = true
  try {
    const info = wizardTmpl.value.detectedInfo
    const summary = `Template: ${wizardTmpl.value.name}, Source: ${wizardTmpl.value.sourceType}, Pages: ${info?.pageCount || 'N/A'}, Fonts: ${(info?.fonts || []).join(', ')}, Sizes(pt): ${(info?.fontSizesPt || []).join(', ')}, HasTOC: ${info?.hasTOC}`
    const res = await aiApi.improve({
      text: summary,
      mode: 'suggest_mapping',
      detail: 'concise',
    })
    aiMappingSuggestion.value = res.improved
  } catch (err: any) {
    uiStore.notify('AI suggestion unavailable: ' + (err.response?.data?.error || err.message), 'error')
  } finally {
    aiSuggesting.value = false
  }
}

async function saveWizard(approve: boolean) {
  if (!wizardTmpl.value) return
  savingWizard.value = true
  try {
    if (approve) wizardTmpl.value.approved = true
    const saved = await templatesApi.update(wizardTmpl.value.id, wizardTmpl.value)
    const idx = templates.value.findIndex(t => t.id === saved.id)
    if (idx !== -1) templates.value[idx] = saved
    uiStore.notify(approve ? 'Template approved & saved!' : 'Template settings saved!', 'success')
    wizardTmpl.value = null
  } catch (err: any) {
    uiStore.notify('Failed to save template: ' + err.message, 'error')
  } finally {
    savingWizard.value = false
  }
}

// Visual Comparison
async function openCompare(tmpl: Template) {
  compareTmpl.value = tmpl
  compareZoom.value = 0.85
  overlayMode.value = false
  pdfPageNum.value = 1
  pdfTotalPages.value = 0
  pdfLoadError.value = ''
  pdfDocProxy = null

  if (tmpl.sourceType === 'pdf' && tmpl.sourceFile) {
    await nextTick()
    try {
      const loadingTask = pdfjsLib.getDocument({ url: templatesApi.sourceUrl(tmpl.id) })
      pdfDocProxy = await loadingTask.promise
      pdfTotalPages.value = pdfDocProxy.numPages
      await renderPDFPage(1)
    } catch (err: any) {
      pdfLoadError.value = 'Could not render source PDF via PDF.js: ' + err.message
    }
  }
}

async function renderPDFPage(num: number) {
  if (!pdfDocProxy || !pdfCanvasRef.value) return
  const page = await pdfDocProxy.getPage(num)
  const viewport = page.getViewport({ scale: compareZoom.value * 1.3 })
  const canvas = pdfCanvasRef.value
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  canvas.height = viewport.height
  canvas.width = viewport.width
  await page.render({ canvasContext: ctx, viewport }).promise
}

async function changeComparePage(delta: number) {
  const next = pdfPageNum.value + delta
  if (next >= 1 && next <= pdfTotalPages.value) {
    pdfPageNum.value = next
    await renderPDFPage(next)
  }
}

async function approveFromCompare() {
  if (!compareTmpl.value) return
  const updated = { ...compareTmpl.value, approved: true }
  try {
    const saved = await templatesApi.update(updated.id, updated)
    const idx = templates.value.findIndex(t => t.id === saved.id)
    if (idx !== -1) templates.value[idx] = saved
    compareTmpl.value = saved
    uiStore.notify(`Template "${saved.name}" approved for future reports!`, 'success')
  } catch (err: any) {
    uiStore.notify('Approval failed: ' + err.message, 'error')
  }
}

const comparePreviewSrc = computed(() =>
  compareTmpl.value ? templatesApi.previewUrl(compareTmpl.value.id) + '?t=' + Date.now() : ''
)

onMounted(() => {
  loadTemplates()
})
</script>

<template>
  <div class="p-6 max-w-7xl mx-auto space-y-6">
    <!-- Header -->
    <div class="bg-white p-5 rounded-2xl shadow-sm border border-gray-100 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          <Layout class="w-6 h-6 text-primary-600" /> Manage Templates
        </h1>
        <p class="text-sm text-gray-500 mt-1">
          Manage ERA & team-specific report templates (.docx / .pdf). Each template keeps isolated assets, fonts, and layout rules.
        </p>
      </div>

      <button @click="openImport" class="btn-primary text-xs flex items-center gap-1.5 shadow-sm">
        <Plus class="w-4 h-4" /> Import Team Template
      </button>
    </div>

    <!-- Templates Grid -->
    <div v-if="loading" class="text-center py-16 text-gray-400">
      <div class="animate-spin w-8 h-8 border-4 border-primary-500 border-t-transparent rounded-full mx-auto mb-3"></div>
      Loading templates...
    </div>

    <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      <div
        v-for="tmpl in templates"
        :key="tmpl.id"
        class="bg-white rounded-2xl p-5 shadow-sm border border-gray-100 hover:shadow-md transition flex flex-col justify-between space-y-4 relative overflow-hidden"
      >
        <div class="h-2.5 w-full absolute top-0 left-0" :style="{ backgroundColor: tmpl.primaryColor || '#262262' }"></div>

        <div class="space-y-3 pt-2">
          <div class="flex items-center justify-between gap-2 flex-wrap">
            <span
              v-if="tmpl.isDefault"
              class="text-[11px] font-bold text-emerald-700 bg-emerald-50 px-2.5 py-0.5 rounded-full border border-emerald-200 flex items-center gap-1"
            >
              <Check class="w-3 h-3" /> Default
            </span>
            <span v-else class="text-[11px] font-semibold uppercase tracking-wide px-2 py-0.5 rounded bg-gray-100 text-gray-600">
              {{ tmpl.sourceType }}
            </span>

            <span
              v-if="tmpl.approved"
              class="text-[11px] font-semibold text-blue-700 bg-blue-50 px-2 py-0.5 rounded-full border border-blue-200 flex items-center gap-1"
            >
              <ShieldCheck class="w-3 h-3" /> Approved
            </span>
            <span
              v-else
              class="text-[11px] font-semibold text-amber-700 bg-amber-50 px-2 py-0.5 rounded-full border border-amber-200 flex items-center gap-1"
            >
              <AlertTriangle class="w-3 h-3" /> Needs Verification
            </span>
          </div>

          <div>
            <div class="flex items-center justify-between gap-2">
              <h3 class="text-base font-bold text-gray-900 truncate">{{ tmpl.name }}</h3>
              <button @click="startRename(tmpl)" class="text-gray-400 hover:text-gray-700" title="Rename">
                <Edit3 class="w-3.5 h-3.5" />
              </button>
            </div>
            <p class="text-xs text-gray-500 mt-1 leading-relaxed line-clamp-2">{{ tmpl.description }}</p>
          </div>

          <!-- Key Typography Summary -->
          <div class="bg-gray-50 rounded-xl p-3 border border-gray-100 space-y-1 text-[11px] text-gray-600">
            <div class="flex justify-between">
              <span>Mode:</span>
              <strong class="text-gray-800">{{ tmpl.formattingMode === 'custom' ? 'Custom (23/16/14pt)' : 'Match Template' }}</strong>
            </div>
            <div class="flex justify-between">
              <span>Project / Task / Body:</span>
              <strong class="text-gray-800">
                {{ tmpl.typography?.projectName?.fontSizePt || 23 }}pt / {{ tmpl.typography?.taskTitle?.fontSizePt || 16 }}pt / {{ tmpl.typography?.taskDescription?.fontSizePt || 14 }}pt
              </strong>
            </div>
            <div class="flex justify-between">
              <span>Isolated Assets:</span>
              <strong class="text-gray-800">{{ tmpl.assets?.extractedFiles?.length || 0 }} file(s)</strong>
            </div>
          </div>
        </div>

        <div class="pt-3 border-t border-gray-100 space-y-2">
          <div class="grid grid-cols-2 gap-2">
            <button @click="openWizard(tmpl)" class="btn-secondary text-xs py-1.5 px-2 flex items-center justify-center gap-1">
              <Sliders class="w-3.5 h-3.5" /> Mapping Wizard
            </button>
            <button @click="openCompare(tmpl)" class="btn-secondary text-xs py-1.5 px-2 flex items-center justify-center gap-1">
              <GitCompare class="w-3.5 h-3.5" /> Compare
            </button>
          </div>
          <div class="flex items-center justify-between gap-2">
            <button
              @click="setAsDefault(tmpl)"
              class="flex-1 btn-primary text-xs py-1.5 px-2"
              :disabled="tmpl.isDefault"
            >
              {{ tmpl.isDefault ? 'Active Default' : 'Set as Default' }}
            </button>
            <button @click="duplicateTemplate(tmpl)" class="p-1.5 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50" title="Duplicate">
              <Copy class="w-3.5 h-3.5" />
            </button>
            <button
              v-if="tmpl.id !== 'era'"
              @click="deleteTemplate(tmpl)"
              class="p-1.5 rounded-lg border border-red-200 text-red-600 hover:bg-red-50"
              title="Delete"
            >
              <Trash2 class="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Import Template Modal -->
    <div v-if="showImportModal" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-xs p-4">
      <div class="bg-white rounded-2xl shadow-xl w-full max-w-lg overflow-hidden">
        <div class="px-6 py-4 border-b border-gray-100 flex items-center justify-between bg-gray-50">
          <h3 class="text-base font-bold text-gray-900">Import Team Template (.docx / .pdf)</h3>
          <button @click="showImportModal = false" class="text-gray-400 hover:text-gray-700">✕</button>
        </div>

        <div class="p-6 space-y-4">
          <input ref="fileInputRef" type="file" accept=".docx,.pdf" class="hidden" @change="onFileSelected" />

          <div
            @click="triggerFileInput"
            @dragover.prevent
            @drop.prevent="onFileDrop"
            class="border-2 border-dashed rounded-xl p-6 text-center transition cursor-pointer"
            :class="selectedFile ? 'border-emerald-400 bg-emerald-50/30' : 'border-gray-200 hover:border-primary-400 bg-gray-50/50'"
          >
            <div v-if="selectedFile">
              <Check class="w-8 h-8 text-emerald-600 mx-auto mb-2" />
              <p class="text-xs font-bold text-emerald-800">{{ selectedFile.name }}</p>
              <p class="text-[11px] text-emerald-600 mt-0.5">{{ (selectedFile.size / 1024).toFixed(1) }} KB • Click to change</p>
            </div>
            <div v-else>
              <UploadCloud class="w-8 h-8 text-primary-500 mx-auto mb-2" />
              <p class="text-xs font-semibold text-gray-700">Click or drag & drop a Word (.docx) or PDF (.pdf) template</p>
              <p class="text-[11px] text-gray-400 mt-0.5">Automatically analyzes page size, fonts, colors & extracts images into isolated storage</p>
            </div>
          </div>

          <div>
            <label class="label">Template Name</label>
            <input v-model="newTemplateName" type="text" class="input-field" placeholder="e.g. QA Team Monthly Report" />
          </div>

          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="label">Primary Brand Color</label>
              <div class="flex items-center gap-2">
                <input v-model="newTemplateColor" type="color" class="w-9 h-9 p-0.5 rounded border cursor-pointer" />
                <input v-model="newTemplateColor" type="text" class="input-field font-mono text-xs" />
              </div>
            </div>
            <div>
              <label class="label">Accent / TOC Color</label>
              <div class="flex items-center gap-2">
                <input v-model="newTemplateAccent" type="color" class="w-9 h-9 p-0.5 rounded border cursor-pointer" />
                <input v-model="newTemplateAccent" type="text" class="input-field font-mono text-xs" />
              </div>
            </div>
          </div>
        </div>

        <div class="px-6 py-4 border-t border-gray-100 flex justify-end gap-3 bg-gray-50">
          <button @click="showImportModal = false" class="btn-secondary text-xs">Cancel</button>
          <button @click="handleImportSubmit" :disabled="importing" class="btn-primary text-xs">
            {{ importing ? 'Analyzing & Importing...' : 'Import & Analyze Template' }}
          </button>
        </div>
      </div>
    </div>

    <!-- Rename Modal -->
    <div v-if="renamingTmpl" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div class="bg-white rounded-2xl shadow-xl w-full max-w-sm p-6 space-y-4">
        <h3 class="text-base font-bold text-gray-900">Rename Template</h3>
        <input v-model="renameValue" type="text" class="input-field" />
        <div class="flex justify-end gap-2">
          <button @click="renamingTmpl = null" class="btn-secondary text-xs">Cancel</button>
          <button @click="confirmRename" class="btn-primary text-xs">Save Name</button>
        </div>
      </div>
    </div>

    <!-- Template Mapping & Typography Wizard Modal -->
    <div v-if="wizardTmpl" class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
      <div class="bg-white rounded-2xl shadow-2xl w-full max-w-5xl max-h-[90vh] flex flex-col overflow-hidden">
        <div class="px-6 py-4 border-b border-gray-100 flex items-center justify-between bg-gray-50">
          <div>
            <h3 class="text-lg font-bold text-gray-900">Template Mapping & Formatting Wizard — {{ wizardTmpl.name }}</h3>
            <p class="text-xs text-gray-500">Configure exact point sizes, page geometry, extracted assets, and field mappings.</p>
          </div>
          <button @click="wizardTmpl = null" class="text-gray-400 hover:text-gray-700 text-lg">✕</button>
        </div>

        <!-- Tabs -->
        <div class="px-6 pt-3 border-b border-gray-100 flex gap-4 bg-white">
          <button
            v-for="tab in [
              { id: 'analysis', label: '1. Analysis & Page Setup' },
              { id: 'typography', label: '2. Typography (pt)' },
              { id: 'mappings', label: '3. Field Mappings' },
              { id: 'assets', label: '4. Isolated Assets' }
            ]"
            :key="tab.id"
            @click="wizardTab = tab.id as any"
            class="pb-2.5 text-xs font-semibold border-b-2 transition"
            :class="wizardTab === tab.id ? 'border-primary-600 text-primary-700' : 'border-transparent text-gray-500 hover:text-gray-800'"
          >
            {{ tab.label }}
          </button>
        </div>

        <div class="p-6 overflow-y-auto flex-1 space-y-5">
          <!-- Tab 1: Analysis & Page Setup -->
          <div v-if="wizardTab === 'analysis'" class="space-y-5">
            <div v-if="wizardTmpl.detectedInfo" class="bg-indigo-50/60 border border-indigo-100 rounded-xl p-4 space-y-2">
              <div class="text-xs font-bold text-indigo-900 uppercase tracking-wider">Automatic Document Analysis</div>
              <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
                <div><span class="text-gray-500">Page Size:</span> <strong>{{ wizardTmpl.detectedInfo.pageWidthPt }} x {{ wizardTmpl.detectedInfo.pageHeightPt }} pt</strong></div>
                <div><span class="text-gray-500">Orientation:</span> <strong class="capitalize">{{ wizardTmpl.detectedInfo.orientation }}</strong></div>
                <div><span class="text-gray-500">Images Extracted:</span> <strong>{{ wizardTmpl.detectedInfo.imagesCount }}</strong></div>
                <div><span class="text-gray-500">TOC Detected:</span> <strong>{{ wizardTmpl.detectedInfo.hasTOC ? 'Yes' : 'No' }}</strong></div>
              </div>
              <div v-if="wizardTmpl.detectedInfo.fonts?.length" class="text-xs">
                <span class="text-gray-500">Detected Fonts:</span> {{ wizardTmpl.detectedInfo.fonts.join(', ') }}
              </div>
              <div v-if="wizardTmpl.detectedInfo.fontSizesPt?.length" class="text-xs">
                <span class="text-gray-500">Detected Sizes (pt):</span> {{ wizardTmpl.detectedInfo.fontSizesPt.join('pt, ') }}pt
              </div>
              <ul v-if="wizardTmpl.detectedInfo.notes?.length" class="text-xs text-indigo-800 list-disc pl-5 pt-1 space-y-0.5">
                <li v-for="(n, idx) in wizardTmpl.detectedInfo.notes" :key="idx">{{ n }}</li>
              </ul>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <label class="label">Formatting Mode</label>
                <select v-model="wizardTmpl.formattingMode" class="input-field">
                  <option value="match_template">Match Uploaded Template</option>
                  <option value="custom">Custom Formatting (23pt / 16pt / 14pt)</option>
                </select>
              </div>
              <div class="flex items-end">
                <button @click="applyCustomERAPreset" class="btn-secondary text-xs w-full">
                  Apply 23pt / 16pt / 14pt Custom Preset
                </button>
              </div>
            </div>

            <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
              <div>
                <label class="label">Width (mm)</label>
                <input v-model.number="wizardTmpl.pageConfig.widthMM" type="number" step="0.5" class="input-field" />
              </div>
              <div>
                <label class="label">Height (mm)</label>
                <input v-model.number="wizardTmpl.pageConfig.heightMM" type="number" step="0.5" class="input-field" />
              </div>
              <div>
                <label class="label">Line Spacing</label>
                <input v-model.number="wizardTmpl.pageConfig.lineSpacing" type="number" step="0.05" class="input-field" />
              </div>
              <div>
                <label class="label">Bullet Indent (mm)</label>
                <input v-model.number="wizardTmpl.pageConfig.bulletIndentMM" type="number" step="0.5" class="input-field" />
              </div>
              <div>
                <label class="label">Top Margin (mm)</label>
                <input v-model.number="wizardTmpl.pageConfig.marginTopMM" type="number" class="input-field" />
              </div>
              <div>
                <label class="label">Bottom Margin (mm)</label>
                <input v-model.number="wizardTmpl.pageConfig.marginBottomMM" type="number" class="input-field" />
              </div>
              <div>
                <label class="label">Left Margin (mm)</label>
                <input v-model.number="wizardTmpl.pageConfig.marginLeftMM" type="number" class="input-field" />
              </div>
              <div>
                <label class="label">Right Margin (mm)</label>
                <input v-model.number="wizardTmpl.pageConfig.marginRightMM" type="number" class="input-field" />
              </div>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 items-center">
              <div>
                <label class="label">Footer Text</label>
                <input v-model="wizardTmpl.pageConfig.footerText" type="text" class="input-field" placeholder="e.g. Public" />
              </div>
              <div>
                <label class="label">Page Numbers</label>
                <label class="flex items-center gap-2 text-xs font-medium mt-2">
                  <input v-model="wizardTmpl.pageConfig.showPageNumbers" type="checkbox" class="rounded" /> Show Page Numbers
                </label>
              </div>
              <div>
                <label class="label">Primary / Accent Colors</label>
                <div class="flex gap-2">
                  <input v-model="wizardTmpl.primaryColor" type="color" class="w-9 h-9 p-0.5 rounded border" />
                  <input v-model="wizardTmpl.accentColor" type="color" class="w-9 h-9 p-0.5 rounded border" />
                </div>
              </div>
            </div>
          </div>

          <!-- Tab 2: Typography (actual pt) -->
          <div v-if="wizardTab === 'typography'" class="space-y-4">
            <div
              v-for="item in [
                { key: 'projectName', label: 'Project Name (Default: Century Gothic, 23 pt, Bold)' },
                { key: 'taskTitle', label: 'Task Title (Default: Times New Roman, 16 pt, Bold)' },
                { key: 'subheading', label: 'Subheadings (Default: Times New Roman, 14 pt, Bold)' },
                { key: 'taskDescription', label: 'Task Description (Default: Times New Roman, 14 pt, Regular)' },
                { key: 'tocTitle', label: 'TOC Heading' },
                { key: 'tocProject', label: 'TOC Project Entry' },
                { key: 'tocTask', label: 'TOC Task Entry' }
              ]"
              :key="item.key"
              class="p-3 bg-gray-50 rounded-xl border border-gray-100 grid grid-cols-1 sm:grid-cols-5 gap-3 items-center"
            >
              <div class="sm:col-span-2 text-xs font-semibold text-gray-800">{{ item.label }}</div>
              <div>
                <input v-model="(wizardTmpl.typography as any)[item.key].fontFamily" type="text" class="input-field text-xs" placeholder="Font Family" />
              </div>
              <div class="flex items-center gap-1.5">
                <input v-model.number="(wizardTmpl.typography as any)[item.key].fontSizePt" type="number" step="0.5" class="input-field text-xs w-20" />
                <span class="text-xs text-gray-500">pt</span>
              </div>
              <div class="flex items-center gap-3">
                <label class="flex items-center gap-1 text-xs">
                  <input v-model="(wizardTmpl.typography as any)[item.key].bold" type="checkbox" class="rounded" /> Bold
                </label>
                <input v-model="(wizardTmpl.typography as any)[item.key].color" type="color" class="w-7 h-7 p-0.5 rounded border" />
              </div>
            </div>
          </div>

          <!-- Tab 3: Field Mappings -->
          <div v-if="wizardTab === 'mappings'" class="space-y-4">
            <div class="flex items-center justify-between flex-wrap gap-2">
              <p class="text-xs text-gray-500">
                Map Cover, Employee Details, TOC, and Repeatable Task sections. For imported PDFs, enable "Mask Existing" to prevent text overlap.
              </p>
              <div class="flex gap-2">
                <button @click="suggestMappingsWithAI" :disabled="aiSuggesting" class="btn-secondary text-xs flex items-center gap-1">
                  <Sparkles class="w-3.5 h-3.5 text-purple-600" /> {{ aiSuggesting ? 'Suggesting...' : 'AI Suggest Mappings' }}
                </button>
                <button @click="addMappingRow" class="btn-primary text-xs flex items-center gap-1">
                  <Plus class="w-3.5 h-3.5" /> Add Field Mapping
                </button>
              </div>
            </div>

            <div v-if="aiMappingSuggestion" class="bg-purple-50 border border-purple-200 rounded-xl p-3 text-xs text-purple-900 whitespace-pre-wrap">
              <div class="font-bold mb-1">AI Mapping Suggestion (Review & Apply Manually):</div>
              {{ aiMappingSuggestion }}
            </div>

            <div class="space-y-2">
              <div
                v-for="(m, idx) in wizardTmpl.fieldMappings"
                :key="m.id"
                class="p-3 bg-gray-50 rounded-xl border border-gray-200 grid grid-cols-2 sm:grid-cols-7 gap-2 items-center text-xs"
              >
                <select v-model="m.fieldType" class="input-field text-xs sm:col-span-2">
                  <option v-for="opt in fieldTypeOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
                </select>
                <input v-model="m.fontFamily" type="text" class="input-field text-xs" placeholder="Font" />
                <input v-model.number="m.fontSizePt" type="number" class="input-field text-xs" placeholder="pt" />
                <input v-model.number="m.pageNumber" type="number" min="1" class="input-field text-xs" placeholder="Page" />
                <label class="flex items-center gap-1 text-[11px]">
                  <input v-model="m.maskExisting" type="checkbox" class="rounded" /> Mask PDF
                </label>
                <button @click="removeMappingRow(idx)" class="text-red-600 hover:text-red-800 justify-self-end">
                  <Trash2 class="w-4 h-4" />
                </button>
              </div>
            </div>
          </div>

          <!-- Tab 4: Isolated Assets -->
          <div v-if="wizardTab === 'assets'" class="space-y-4">
            <p class="text-xs text-gray-500">
              Assets stored strictly under <code>templates/{{ wizardTmpl.id }}/assets/</code>. Never shared across teams.
            </p>
            <div v-if="!wizardTmpl.assets?.extractedFiles?.length" class="text-center py-8 text-xs text-gray-400 bg-gray-50 rounded-xl border border-dashed">
              No extracted image assets in this template.
            </div>
            <div v-else class="grid grid-cols-2 sm:grid-cols-4 gap-4">
              <div
                v-for="file in wizardTmpl.assets.extractedFiles"
                :key="file"
                class="border rounded-xl p-3 bg-gray-50 space-y-2 text-center"
              >
                <img :src="templatesApi.assetUrl(wizardTmpl.id, file)" class="h-24 mx-auto object-contain rounded bg-white p-1 border" />
                <div class="text-[11px] font-mono truncate">{{ file }}</div>
                <div class="flex gap-1 justify-center">
                  <button
                    @click="wizardTmpl.assets.coverHeroImage = file"
                    class="text-[10px] px-2 py-0.5 rounded border"
                    :class="wizardTmpl.assets.coverHeroImage === file ? 'bg-primary-600 text-white border-primary-600' : 'bg-white text-gray-700'"
                  >
                    Cover Hero
                  </button>
                  <button
                    @click="wizardTmpl.assets.logoImage = file"
                    class="text-[10px] px-2 py-0.5 rounded border"
                    :class="wizardTmpl.assets.logoImage === file ? 'bg-primary-600 text-white border-primary-600' : 'bg-white text-gray-700'"
                  >
                    Logo
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="px-6 py-4 border-t border-gray-100 flex justify-between items-center bg-gray-50">
          <button @click="wizardTmpl = null" class="btn-secondary text-xs">Close</button>
          <div class="flex gap-2">
            <button @click="saveWizard(false)" :disabled="savingWizard" class="btn-secondary text-xs">Save Draft Settings</button>
            <button @click="saveWizard(true)" :disabled="savingWizard" class="btn-primary text-xs flex items-center gap-1">
              <ShieldCheck class="w-3.5 h-3.5" /> Save & Approve Template
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Side-by-Side Visual Comparison Modal -->
    <div v-if="compareTmpl" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div class="bg-white rounded-2xl shadow-2xl w-full max-w-7xl h-[92vh] flex flex-col overflow-hidden">
        <div class="px-6 py-3.5 border-b border-gray-100 flex items-center justify-between bg-gray-50 flex-wrap gap-3">
          <div>
            <h3 class="text-base font-bold text-gray-900 flex items-center gap-2">
              <GitCompare class="w-4 h-4 text-primary-600" /> Compare Template — {{ compareTmpl.name }}
            </h3>
            <p class="text-xs text-gray-500">Left: Original Uploaded File • Right: Generated Report Preview</p>
          </div>

          <div class="flex items-center gap-3">
            <label class="flex items-center gap-1.5 text-xs font-medium bg-white px-2.5 py-1.5 rounded-lg border">
              <input v-model="overlayMode" type="checkbox" class="rounded" />
              <Layers class="w-3.5 h-3.5 text-primary-600" /> Overlay Diff Mode
            </label>
            <input v-if="overlayMode" v-model.number="overlayOpacity" type="range" min="0.1" max="0.9" step="0.1" class="w-24" />
            <button @click="approveFromCompare" class="btn-primary text-xs flex items-center gap-1">
              <ShieldCheck class="w-3.5 h-3.5" /> {{ compareTmpl.approved ? 'Approved ✓' : 'Approve Template' }}
            </button>
            <button @click="compareTmpl = null" class="text-gray-400 hover:text-gray-700 text-lg px-2">✕</button>
          </div>
        </div>

        <div class="flex-1 grid grid-cols-1 lg:grid-cols-2 divide-y lg:divide-y-0 lg:divide-x divide-gray-200 overflow-hidden">
          <!-- Left: Original Source -->
          <div class="flex flex-col bg-gray-100 overflow-hidden">
            <div class="px-4 py-2 bg-white border-b flex items-center justify-between text-xs">
              <span class="font-bold text-gray-700">Original Template ({{ compareTmpl.sourceType.toUpperCase() }})</span>
              <div v-if="pdfTotalPages > 0" class="flex items-center gap-2">
                <button @click="changeComparePage(-1)" :disabled="pdfPageNum <= 1" class="p-1 rounded border"><ChevronLeft class="w-3.5 h-3.5" /></button>
                <span>Page {{ pdfPageNum }} / {{ pdfTotalPages }}</span>
                <button @click="changeComparePage(1)" :disabled="pdfPageNum >= pdfTotalPages" class="p-1 rounded border"><ChevronRight class="w-3.5 h-3.5" /></button>
              </div>
            </div>
            <div class="flex-1 overflow-auto p-4 flex items-start justify-center">
              <canvas v-if="compareTmpl.sourceType === 'pdf'" ref="pdfCanvasRef" class="shadow-lg bg-white max-w-full" />
              <div v-else class="bg-white rounded-xl p-6 max-w-md w-full shadow-sm space-y-3 text-xs">
                <div class="font-bold text-sm text-gray-900">
                  {{ compareTmpl.sourceType === 'era-builtin' ? 'Built-in Canonical ERA Reference' : 'Uploaded DOCX Template Analysis' }}
                </div>
                <p class="text-gray-500">
                  Fonts: {{ compareTmpl.detectedInfo?.fonts?.join(', ') || 'Century Gothic, Times New Roman' }}
                </p>
                <p class="text-gray-500">
                  Page: {{ compareTmpl.pageConfig.widthMM }}mm x {{ compareTmpl.pageConfig.heightMM }}mm ({{ compareTmpl.pageConfig.orientation }})
                </p>
                <div v-if="compareTmpl.assets?.extractedFiles?.length" class="grid grid-cols-2 gap-2 pt-2">
                  <img
                    v-for="f in compareTmpl.assets.extractedFiles"
                    :key="f"
                    :src="templatesApi.assetUrl(compareTmpl.id, f)"
                    class="h-20 object-contain border rounded p-1"
                  />
                </div>
              </div>
              <div v-if="pdfLoadError" class="text-xs text-red-600 p-4">{{ pdfLoadError }}</div>
            </div>
          </div>

          <!-- Right: Generated Preview -->
          <div class="flex flex-col bg-gray-100 overflow-hidden relative">
            <div class="px-4 py-2 bg-white border-b flex items-center justify-between text-xs">
              <span class="font-bold text-gray-700">Generated Report Preview</span>
              <span class="text-gray-400">Uses {{ compareTmpl.formattingMode === 'custom' ? 'Custom 23/16/14pt' : 'Template Styles' }}</span>
            </div>
            <div class="flex-1 overflow-hidden relative">
              <iframe
                :src="comparePreviewSrc"
                class="w-full h-full border-0"
                :style="overlayMode ? { opacity: overlayOpacity } : {}"
                title="Generated Template Comparison"
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
