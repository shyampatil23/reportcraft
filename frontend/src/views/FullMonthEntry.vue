<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  FileText, Sparkles, Save, Trash2, ArrowRight, Check,
  ChevronDown, ChevronUp, AlertTriangle, Layers, Edit3, Plus, CornerDownRight
} from 'lucide-vue-next'
import { format } from 'date-fns'
import { fullMonthApi, aiApi, reportsApi, templatesApi, editorApi } from '@/api'
import { useTasksStore } from '@/stores/tasks'
import { useUIStore } from '@/stores/ui'
import type { ParsedTask, Task } from '@/types'

const router = useRouter()
const tasksStore = useTasksStore()
const uiStore = useUIStore()

const currentMonth = ref(format(new Date(), 'yyyy-MM'))
const activeTab = ref<'editor' | 'review'>('editor')

const rawText = ref('')
const isParsing = ref(false)
const isSaving = ref(false)
const isImproving = ref(false)
const lastSavedTime = ref<string | null>(null)

// Parsed tasks from offline / AI parser
const parsedTasks = ref<ParsedTask[]>([])
const parsedProjects = ref<string[]>([])

// Sample monthly work placeholder text
const placeholderText = `EMACloudContentManager

Translated Video Integration API
Enhanced the CDP API to support multiple translated videos within one request. Updated the payload and response structures.

Prevention of Parallel API Requests
Implemented mutex protection to prevent concurrent integration requests.

ERA 3.0

Simulation Improvements
Removed the Show Targets option and improved simulation behavior.`

// Group parsed tasks by project
const groupedTasks = computed(() => {
  const map: Record<string, ParsedTask[]> = {}
  for (const t of parsedTasks.value) {
    const proj = t.project || 'Unassigned Project'
    if (!map[proj]) map[proj] = []
    map[proj].push(t)
  }
  return map
})

const totalParsedTasks = computed(() => parsedTasks.value.length)
const ambiguousCount = computed(() => parsedTasks.value.filter(t => t.isAmbiguous).length)

// Load draft on mount or month change
async function loadDraft() {
  const [year, month] = currentMonth.value.split('-')
  try {
    const draft = await fullMonthApi.get(year, month)
    if (draft && draft.rawText) {
      rawText.value = draft.rawText
      lastSavedTime.value = draft.savedAt ? format(new Date(draft.savedAt), 'HH:mm') : null
    }
  } catch {
    // If no draft exists yet, it's fine
  }
}

async function saveDraft() {
  const [year, month] = currentMonth.value.split('-')
  isSaving.value = true
  try {
    await fullMonthApi.save(year, month, {
      month: currentMonth.value,
      rawText: rawText.value,
    })
    lastSavedTime.value = format(new Date(), 'HH:mm')
    uiStore.notify('Draft saved locally')
  } catch (err: any) {
    uiStore.notify('Failed to save draft', 'error')
  } finally {
    isSaving.value = false
  }
}

const isGenerating = ref(false)

async function generateMonthlyReport() {
  if (!rawText.value.trim()) {
    uiStore.notify('Please paste or write your monthly work content first', 'error')
    return
  }

  const [year, month] = currentMonth.value.split('-')
  isGenerating.value = true
  isParsing.value = true
  try {
    // 1. Preserve original raw input in local JSON
    await fullMonthApi.save(year, month, {
      month: currentMonth.value,
      rawText: rawText.value,
    })
    lastSavedTime.value = format(new Date(), 'HH:mm')

    // 2. Analyze and organize tasks
    const res = await fullMonthApi.parse(rawText.value)
    parsedTasks.value = res.tasks || []
    parsedProjects.value = res.projects || []

    if (parsedTasks.value.length === 0) {
      uiStore.notify('No tasks could be extracted. Please check your text format.', 'error')
      return
    }

    // 3. Auto-save structured tasks into local JSON
    const finalTasks: Task[] = parsedTasks.value.map((pt, i) => {
      const day = Math.min(Math.max(i + 1, 1), 28)
      const dateStr = `${year}-${month}-${String(day).padStart(2, '0')}`
      return {
        id: pt.id || `fm-${Date.now()}-${i}`,
        date: dateStr,
        project: pt.project || 'General',
        title: pt.title || 'Untitled Task',
        shortDescription: pt.shortDescription || '',
        detailedDescription: pt.detailedDescription || '',
        issueIdentified: pt.issueIdentified || '',
        activitiesPerformed: pt.activitiesPerformed || pt.detailedDescription || '',
        solutionImplemented: pt.solutionImplemented || '',
        outcome: pt.outcome || '',
        status: 'completed',
        tags: ['monthly-import'],
        source: 'fullmonth',
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      }
    })
    await tasksStore.bulkImport(finalTasks)

    // 4. Protect existing manual document edits if present
    let forceOverwrite = false
    let skipRegen = false
    try {
      const st = await editorApi.status(year, month)
      if (st.hasSavedEdits || st.hasDraft) {
        const overwrite = window.confirm(
          'Manual document edits exist for this month. Regenerating will replace your manual edits (a backup copy will be saved).\n\nClick OK to regenerate, or Cancel to keep your existing edited report.'
        )
        if (!overwrite) {
          skipRegen = true
        } else {
          forceOverwrite = true
        }
      }
    } catch {
      // ignore if no status yet
    }

    // 5. Generate report immediately using default template
    if (!skipRegen) {
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
        // fallback to era
      }
      await reportsApi.generate(
        currentMonth.value,
        finalTasks.map(t => t.id),
        defaultTmplId,
        defaultMode,
        forceOverwrite
      )
    }

    const ambiguousList = parsedTasks.value
      .filter(t => t.isAmbiguous)
      .map(t => t.title || 'Untitled Task')
      .slice(0, 5)
      .join(', ')

    uiStore.notify(`Generated monthly report with ${finalTasks.length} tasks!`, 'success')

    // 6. Navigate straight to Report Preview (Step 5)
    await router.push({
      path: '/generate-report',
      query: {
        month: currentMonth.value,
        step: '5',
        ...(ambiguousList ? { ambiguous: ambiguousList } : {}),
      },
    })
  } catch (err: any) {
    uiStore.notify('Failed to generate monthly report: ' + (err?.message || 'Unknown error'), 'error')
  } finally {
    isGenerating.value = false
    isParsing.value = false
  }
}

async function analyzeAndOrganize() {
  if (!rawText.value.trim()) {
    uiStore.notify('Please paste or write your monthly work content first', 'error')
    return
  }

  isParsing.value = true
  try {
    const res = await fullMonthApi.parse(rawText.value)
    parsedTasks.value = res.tasks || []
    parsedProjects.value = res.projects || []
    activeTab.value = 'review'
    uiStore.notify(`Extracted ${parsedTasks.value.length} tasks across ${parsedProjects.value.length} projects!`)
  } catch (err: any) {
    uiStore.notify('Error parsing text: ' + err.message, 'error')
  } finally {
    isParsing.value = false
  }
}

async function improveWithAI() {
  if (!rawText.value.trim()) {
    uiStore.notify('Please enter some text to improve', 'info')
    return
  }

  isImproving.value = true
  try {
    const res = await aiApi.improve({
      text: rawText.value,
      mode: 'improve',
      detail: 'standard',
    })
    if (res && res.improved) {
      rawText.value = res.improved
      uiStore.notify('Text enhanced with AI!')
    }
  } catch (err: any) {
    const msg = err.response?.data?.error || err.message || 'AI is unavailable offline.'
    uiStore.notify(msg, 'error')
  } finally {
    isImproving.value = false
  }
}

function clearText() {
  uiStore.confirm('Clear Content', 'Are you sure you want to clear the editor? Any unsaved changes will be lost.', () => {
    rawText.value = ''
    parsedTasks.value = []
    uiStore.notify('Editor cleared')
  })
}

// Review manipulations
function removeTask(id: string) {
  parsedTasks.value = parsedTasks.value.filter(t => t.id !== id)
}

function updateProjectName(oldProj: string, newProj: string) {
  if (!newProj.trim()) return
  for (const t of parsedTasks.value) {
    if (t.project === oldProj) {
      t.project = newProj.trim()
    }
  }
}

function promptRenameProject(project: string) {
  const newP = window.prompt('Rename project for these tasks:', project)
  if (newP) updateProjectName(project, newP)
}

// Confirm and Import to My Tasks / Daily Tasks
async function confirmAndAddToReport() {
  if (parsedTasks.value.length === 0) {
    uiStore.notify('No tasks to import', 'error')
    return
  }

  // Convert ParsedTasks to Task objects
  const [year, month] = currentMonth.value.split('-')
  const defaultDate = `${year}-${month}-15`

  const finalTasks: Task[] = parsedTasks.value.map((pt, i) => {
    // Distribute date reasonably through the month or set default
    const day = Math.min(Math.max(i + 1, 1), 28)
    const dateStr = `${year}-${month}-${String(day).padStart(2, '0')}`

    return {
      id: pt.id || `fm-${Date.now()}-${i}`,
      date: dateStr,
      project: pt.project || 'General',
      title: pt.title || 'Untitled Task',
      shortDescription: pt.shortDescription || '',
      detailedDescription: pt.detailedDescription || '',
      issueIdentified: pt.issueIdentified || '',
      activitiesPerformed: pt.activitiesPerformed || pt.detailedDescription || '',
      solutionImplemented: pt.solutionImplemented || '',
      outcome: pt.outcome || '',
      status: 'completed',
      tags: ['monthly-import'],
      source: 'fullmonth',
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    }
  })

  try {
    await tasksStore.bulkImport(finalTasks)
    // Also save raw text draft
    await saveDraft()
    uiStore.notify(`Successfully confirmed & added ${finalTasks.length} tasks to ${currentMonth.value}!`)
    // Optionally stay or notify
  } catch (err: any) {
    uiStore.notify('Failed to save imported tasks: ' + err.message, 'error')
  }
}

onMounted(() => {
  loadDraft()
})
</script>

<template>
  <div class="p-6 max-w-7xl mx-auto space-y-6">
    <!-- Header with Month Selector and Mode Tabs -->
    <div class="bg-white p-5 rounded-2xl shadow-sm border border-gray-100 flex flex-col md:flex-row md:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          <Layers class="w-6 h-6 text-primary-600" /> Full Month Entry
        </h1>
        <p class="text-sm text-gray-500 mt-1">
          Paste your entire month's notes, commits, or ticket logs. Automatic offline parsing detects projects and tasks.
        </p>
      </div>

      <div class="flex items-center gap-3">
        <input
          type="month"
          v-model="currentMonth"
          @change="loadDraft"
          class="input-field w-auto text-sm"
        />

        <div class="flex bg-gray-100 p-1 rounded-xl">
          <button
            @click="activeTab = 'editor'"
            :class="[
              'px-4 py-2 rounded-lg text-xs font-bold transition select-none',
              activeTab === 'editor' ? 'bg-white text-primary-700 shadow-xs' : 'text-gray-600 hover:text-gray-900'
            ]"
          >
            1. Paste & Edit
          </button>
          <button
            @click="activeTab = 'review'"
            :class="[
              'px-4 py-2 rounded-lg text-xs font-bold transition flex items-center gap-1.5 select-none',
              activeTab === 'review' ? 'bg-white text-primary-700 shadow-xs' : 'text-gray-600 hover:text-gray-900'
            ]"
          >
            2. Structured Review
            <span v-if="totalParsedTasks > 0" class="w-5 h-5 rounded-full bg-primary-100 text-primary-800 text-[10px] flex items-center justify-center font-bold">
              {{ totalParsedTasks }}
            </span>
          </button>
        </div>
      </div>
    </div>

    <!-- TAB 1: PASTE MONTHLY WORK -->
    <div v-show="activeTab === 'editor'" class="space-y-4">
      <div class="bg-white rounded-2xl shadow-sm border border-gray-100 p-6 space-y-4">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <h2 class="text-base font-bold text-gray-900">Paste Your Complete Monthly Work</h2>
            <span v-if="lastSavedTime" class="text-xs text-gray-400">
              Draft saved at {{ lastSavedTime }}
            </span>
          </div>

          <div class="flex items-center gap-2">
            <button
              @click="improveWithAI"
              :disabled="isImproving"
              class="btn-secondary text-xs flex items-center gap-1.5 border-purple-200 text-purple-700 hover:bg-purple-50"
            >
              <Sparkles class="w-3.5 h-3.5 text-purple-600" />
              {{ isImproving ? 'Improving...' : 'Improve with AI' }}
            </button>
            <button
              @click="saveDraft"
              :disabled="isSaving"
              class="btn-secondary text-xs flex items-center gap-1.5"
            >
              <Save class="w-3.5 h-3.5 text-gray-600" />
              {{ isSaving ? 'Saving...' : 'Save Draft' }}
            </button>
            <button
              @click="clearText"
              class="btn-ghost text-xs text-red-600 hover:bg-red-50 flex items-center gap-1.5"
            >
              <Trash2 class="w-3.5 h-3.5" /> Clear
            </button>
          </div>
        </div>

        <textarea
          v-model="rawText"
          :placeholder="placeholderText"
          rows="18"
          class="w-full p-4 rounded-xl border border-gray-200 focus:outline-hidden focus:ring-2 focus:ring-primary-500 font-mono text-sm leading-relaxed text-gray-800 placeholder-gray-400 bg-gray-50/30"
        ></textarea>

        <div class="flex flex-col sm:flex-row items-center justify-between gap-4 pt-2">
          <p class="text-xs text-gray-500">
            💡 <strong>Smart Heading Detection:</strong> Put Project Names as standalone headings, followed by task titles and paragraphs. Works 100% offline!
          </p>

          <div class="flex items-center gap-2">
            <button
              @click="analyzeAndOrganize"
              :disabled="isParsing || isGenerating"
              class="btn-secondary py-2.5 px-4 text-xs flex items-center gap-1.5"
            >
              <Layers class="w-4 h-4" />
              {{ isParsing && !isGenerating ? 'Analyzing...' : 'Review Structure' }}
            </button>
            <button
              @click="generateMonthlyReport"
              :disabled="isGenerating"
              class="btn-primary py-2.5 px-6 text-sm flex items-center gap-2 shadow-sm font-semibold"
            >
              <Sparkles class="w-4 h-4" />
              {{ isGenerating ? 'Generating Report...' : 'Generate Monthly Report' }}
              <ArrowRight class="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- TAB 2: STRUCTURED REVIEW -->
    <div v-show="activeTab === 'review'" class="space-y-6">
      <!-- Ambiguity Notice if any -->
      <div v-if="ambiguousCount > 0" class="bg-amber-50 border border-amber-200 rounded-xl p-4 flex items-start gap-3">
        <AlertTriangle class="w-5 h-5 text-amber-600 flex-shrink-0 mt-0.5" />
        <div class="text-xs text-amber-800">
          <strong class="font-bold">Attention Needed:</strong> {{ ambiguousCount }} tasks had ambiguous headings or missing project associations. Please review and update them below before confirming.
        </div>
      </div>

      <!-- Empty state for review -->
      <div v-if="totalParsedTasks === 0" class="bg-white rounded-2xl p-12 text-center shadow-sm border border-gray-100">
        <FileText class="w-12 h-12 text-gray-300 mx-auto mb-3" />
        <h3 class="text-base font-bold text-gray-800">No Structured Tasks Yet</h3>
        <p class="text-xs text-gray-500 max-w-md mx-auto mt-1 mb-4">
          Return to the "Paste & Edit" tab and click "Analyze and Organize Tasks" to automatically extract items.
        </p>
        <button @click="activeTab = 'editor'" class="btn-primary text-xs">
          Go to Paste & Edit
        </button>
      </div>

      <!-- Extracted Tasks Grouped by Project -->
      <div v-else class="space-y-6">
        <div
          v-for="(tasks, project) in groupedTasks"
          :key="project"
          class="bg-white rounded-2xl shadow-sm border border-gray-100 overflow-hidden"
        >
          <!-- Project Header Banner -->
          <div class="bg-gradient-to-r from-primary-700 to-violet-700 px-6 py-3.5 text-white flex items-center justify-between">
            <div class="flex items-center gap-3">
              <span class="font-bold text-sm tracking-wide">{{ project }}</span>
              <span class="text-xs bg-white/20 px-2.5 py-0.5 rounded-full font-semibold">
                {{ tasks.length }} {{ tasks.length === 1 ? 'task' : 'tasks' }}
              </span>
            </div>

            <!-- Inline Project Rename -->
            <button
              @click="promptRenameProject(project)"
              class="text-xs text-white/80 hover:text-white flex items-center gap-1 font-medium bg-white/10 px-2 py-1 rounded"
            >
              <Edit3 class="w-3 h-3" /> Rename Project
            </button>
          </div>

          <!-- Tasks under this project -->
          <div class="p-6 space-y-4 divide-y divide-gray-100">
            <div
              v-for="task in tasks"
              :key="task.id"
              class="pt-4 first:pt-0 space-y-3"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="flex-1 space-y-1">
                  <div class="flex items-center gap-2">
                    <input
                      v-model="task.title"
                      type="text"
                      class="font-bold text-sm text-gray-900 border-b border-transparent hover:border-gray-300 focus:border-primary-500 focus:outline-hidden px-1 py-0.5 w-full bg-transparent transition"
                      placeholder="Task Title"
                    />
                    <span v-if="task.isAmbiguous" class="text-[10px] bg-amber-100 text-amber-800 px-2 py-0.5 rounded-full font-bold flex-shrink-0">
                      Ambiguous
                    </span>
                  </div>
                  <p v-if="task.ambiguityReason" class="text-[11px] text-amber-600 px-1">
                    {{ task.ambiguityReason }}
                  </p>
                </div>

                <button
                  @click="removeTask(task.id)"
                  class="p-1.5 text-gray-400 hover:text-red-600 rounded-lg hover:bg-red-50 transition"
                  title="Remove this task"
                >
                  <Trash2 class="w-4 h-4" />
                </button>
              </div>

              <!-- Detailed fields edit grid -->
              <div class="grid grid-cols-1 md:grid-cols-2 gap-3 bg-gray-50/70 p-3.5 rounded-xl border border-gray-100 text-xs">
                <div>
                  <label class="block font-semibold text-gray-700 mb-1">Activities Performed / Details:</label>
                  <textarea
                    v-model="task.activitiesPerformed"
                    rows="3"
                    class="w-full p-2 rounded-lg border border-gray-200 bg-white text-xs leading-relaxed focus:ring-1 focus:ring-primary-500 focus:outline-hidden"
                    placeholder="Activities carried out..."
                  ></textarea>
                </div>

                <div>
                  <label class="block font-semibold text-gray-700 mb-1">Solution Implemented:</label>
                  <textarea
                    v-model="task.solutionImplemented"
                    rows="3"
                    class="w-full p-2 rounded-lg border border-gray-200 bg-white text-xs leading-relaxed focus:ring-1 focus:ring-primary-500 focus:outline-hidden"
                    placeholder="Solution or fix..."
                  ></textarea>
                </div>

                <div>
                  <label class="block font-semibold text-gray-700 mb-1">Issue Identified (Optional):</label>
                  <textarea
                    v-model="task.issueIdentified"
                    rows="2"
                    class="w-full p-2 rounded-lg border border-gray-200 bg-white text-xs leading-relaxed focus:ring-1 focus:ring-primary-500 focus:outline-hidden"
                    placeholder="Issue background..."
                  ></textarea>
                </div>

                <div>
                  <label class="block font-semibold text-gray-700 mb-1">Outcome (Optional):</label>
                  <textarea
                    v-model="task.outcome"
                    rows="2"
                    class="w-full p-2 rounded-lg border border-gray-200 bg-white text-xs leading-relaxed focus:ring-1 focus:ring-primary-500 focus:outline-hidden"
                    placeholder="Measurable result..."
                  ></textarea>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Review Action Bar -->
        <div class="flex items-center justify-between bg-white p-5 rounded-2xl shadow-sm border border-gray-100">
          <button @click="activeTab = 'editor'" class="btn-secondary text-sm">
            ← Back to Editor
          </button>

          <button
            @click="confirmAndAddToReport"
            class="btn-primary py-2.5 px-6 text-sm flex items-center gap-2 shadow-sm font-semibold"
          >
            <Check class="w-4 h-4" /> Confirm & Add to Report
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
