<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useEditor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Underline from '@tiptap/extension-underline'
import { TextStyle } from '@tiptap/extension-text-style'
import FontFamily from '@tiptap/extension-font-family'
import Color from '@tiptap/extension-color'
import TextAlign from '@tiptap/extension-text-align'
import Image from '@tiptap/extension-image'
import { Table } from '@tiptap/extension-table'
import TableRow from '@tiptap/extension-table-row'
import TableCell from '@tiptap/extension-table-cell'
import TableHeader from '@tiptap/extension-table-header'
import { Extension, Node, mergeAttributes } from '@tiptap/core'
import {
  Bold, Italic, Underline as UnderlineIcon, AlignLeft, AlignCenter, AlignRight, AlignJustify,
  List, ListOrdered, Undo2, Redo2, Scissors, Copy, ClipboardPaste,
  ImagePlus, Table as TableIcon, FilePlus2, Trash2, Save, Check, RotateCcw,
  FileText, RefreshCw, IndentIncrease, IndentDecrease, SlidersHorizontal
} from 'lucide-vue-next'
import { editorApi, type EditedDocument, type EditorPageSettings, type EditorStatus } from '../api'
import { useUIStore } from '../stores/ui'

const props = defineProps<{
  year: string
  month: string
}>()

const emit = defineEmits<{
  (e: 'saved', status: EditorStatus): void
  (e: 'status-change', status: EditorStatus): void
}>()

const uiStore = useUIStore()

// ─── Custom Tiptap Extensions ────────────────────────────────────────────────

const FontSize = Extension.create({
  name: 'fontSize',
  addOptions() {
    return { types: ['textStyle'] }
  },
  addGlobalAttributes() {
    return [
      {
        types: this.options.types,
        attributes: {
          fontSize: {
            default: null,
            parseHTML: element => element.style.fontSize || null,
            renderHTML: attributes => {
              if (!attributes.fontSize) return {}
              return { style: `font-size: ${attributes.fontSize}` }
            },
          },
        },
      },
    ]
  },
  addCommands() {
    return {
      setFontSize: (fontSize: string) => ({ chain }: any) =>
        chain().setMark('textStyle', { fontSize }).run(),
      unsetFontSize: () => ({ chain }: any) =>
        chain().setMark('textStyle', { fontSize: null }).removeEmptyTextStyle().run(),
    } as any
  },
})

const ParagraphLayout = Extension.create({
  name: 'paragraphLayout',
  addOptions() {
    return { types: ['paragraph', 'heading'] }
  },
  addGlobalAttributes() {
    return [
      {
        types: this.options.types,
        attributes: {
          lineHeight: {
            default: null,
            parseHTML: el => el.style.lineHeight || null,
            renderHTML: attrs => (attrs.lineHeight ? { style: `line-height: ${attrs.lineHeight}` } : {}),
          },
          marginTop: {
            default: null,
            parseHTML: el => el.style.marginTop || null,
            renderHTML: attrs => (attrs.marginTop ? { style: `margin-top: ${attrs.marginTop}` } : {}),
          },
          marginBottom: {
            default: null,
            parseHTML: el => el.style.marginBottom || null,
            renderHTML: attrs => (attrs.marginBottom ? { style: `margin-bottom: ${attrs.marginBottom}` } : {}),
          },
          indentPx: {
            default: 0,
            parseHTML: el => {
              const ml = parseFloat(el.style.marginLeft || '0')
              return Number.isFinite(ml) ? ml : 0
            },
            renderHTML: attrs => (attrs.indentPx > 0 ? { style: `margin-left: ${attrs.indentPx}px` } : {}),
          },
        },
      },
    ]
  },
})

const PageBreak = Node.create({
  name: 'pageBreak',
  group: 'block',
  atom: true,
  selectable: true,
  parseHTML() {
    return [{ tag: 'div[data-page-break="true"]' }, { tag: 'hr.page-break' }]
  },
  renderHTML({ HTMLAttributes }) {
    return [
      'div',
      mergeAttributes(HTMLAttributes, { 'data-page-break': 'true', class: 'page-break-marker', contenteditable: 'false' }),
      ['span', {}, 'Manual Page Break'],
    ]
  },
  addCommands() {
    return {
      insertPageBreak: () => ({ chain }: any) =>
        chain().focus().insertContent({ type: 'pageBreak' }).run(),
    } as any
  },
})

const ResizableImage = Image.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      width: {
        default: '420px',
        parseHTML: el => el.style.width || el.getAttribute('width') || '420px',
        renderHTML: attrs => ({
          style: `width: ${attrs.width || '420px'}; height: auto; display: block; ${
            attrs.align === 'center'
              ? 'margin-left: auto; margin-right: auto;'
              : attrs.align === 'right'
              ? 'margin-left: auto; margin-right: 0;'
              : 'margin-left: 0; margin-right: auto;'
          }`,
        }),
      },
      align: {
        default: 'left',
        parseHTML: el => el.getAttribute('data-align') || 'left',
        renderHTML: attrs => ({ 'data-align': attrs.align || 'left' }),
      },
    }
  },
})

// ─── State ───────────────────────────────────────────────────────────────────

const loadingDoc = ref(false)
const savingDraft = ref(false)
const savingFinal = ref(false)
const openingWord = ref(false)
const syncingWord = ref(false)
const showPageSetup = ref(false)
const wordLaunchedPath = ref('')
const status = ref<EditorStatus>({ hasDraft: false, hasSavedEdits: false })

const pageSettings = ref<EditorPageSettings>({
  pageSize: 'Letter',
  marginTopMM: 12.7,
  marginRightMM: 20.32,
  marginBottomMM: 12.7,
  marginLeftMM: 20.32,
  headerText: '',
  footerText: 'Public',
})

const selectedFont = ref('Times New Roman')
const selectedFontSize = ref('14pt')
const selectedColor = ref('#1f2937')
const selectedLineHeight = ref('1.35')
const imageInputRef = ref<HTMLInputElement | null>(null)
const editorHostRef = ref<HTMLElement | null>(null)
const estimatedPages = ref(1)
const pageBreakGuides = ref<number[]>([])

// 96 DPI conversion: 1 mm = 3.779527559 px
const MM_TO_PX = 3.779527559

const pageDimensionsPx = computed(() => {
  const isA4 = pageSettings.value.pageSize === 'A4'
  const wMM = isA4 ? 210 : 215.9
  const hMM = isA4 ? 297 : 279.4
  return {
    width: Math.round(wMM * MM_TO_PX),
    height: Math.round(hMM * MM_TO_PX),
    padTop: Math.round((pageSettings.value.marginTopMM || 12.7) * MM_TO_PX),
    padRight: Math.round((pageSettings.value.marginRightMM || 20.32) * MM_TO_PX),
    padBottom: Math.round((pageSettings.value.marginBottomMM || 12.7) * MM_TO_PX),
    padLeft: Math.round((pageSettings.value.marginLeftMM || 20.32) * MM_TO_PX),
  }
})

// ─── Tiptap Instance ─────────────────────────────────────────────────────────

const editor = useEditor({
  content: '<p>Loading document...</p>',
  extensions: [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
    }),
    Underline,
    TextStyle,
    FontFamily,
    FontSize,
    Color,
    TextAlign.configure({ types: ['heading', 'paragraph'] }),
    ParagraphLayout,
    PageBreak,
    ResizableImage.configure({ allowBase64: true }),
    Table.configure({ resizable: true }),
    TableRow,
    TableHeader,
    TableCell,
  ],
  onUpdate: () => {
    schedulePageMeasure()
  },
})

let measureTimer: ReturnType<typeof setTimeout> | null = null
function schedulePageMeasure() {
  if (measureTimer) clearTimeout(measureTimer)
  measureTimer = setTimeout(recomputePageGuides, 150)
}

function recomputePageGuides() {
  const host = editorHostRef.value
  if (!host) return
  const pm = host.querySelector('.ProseMirror') as HTMLElement | null
  if (!pm) return

  const dims = pageDimensionsPx.value
  const usablePageHeight = Math.max(300, dims.height - dims.padTop - dims.padBottom)
  const guides: number[] = []
  let currentPageTop = 0
  let pageCount = 1

  const children = Array.from(pm.children) as HTMLElement[]
  for (const child of children) {
    if (child.getAttribute('data-page-break') === 'true') {
      const breakY = child.offsetTop + child.offsetHeight / 2
      guides.push(breakY)
      currentPageTop = child.offsetTop + child.offsetHeight
      pageCount++
      continue
    }
    const bottom = child.offsetTop + child.offsetHeight
    while (bottom - currentPageTop > usablePageHeight) {
      const guideY = currentPageTop + usablePageHeight
      guides.push(guideY)
      currentPageTop = guideY
      pageCount++
    }
  }
  estimatedPages.value = Math.max(1, pageCount)
  pageBreakGuides.value = guides
}

// ─── Load / Save / Word Actions ──────────────────────────────────────────────

async function loadDocument() {
  loadingDoc.value = true
  try {
    const res = await editorApi.get(props.year, props.month)
    status.value = res.status
    emit('status-change', res.status)
    if (res.document?.pageSettings) {
      pageSettings.value = { ...pageSettings.value, ...res.document.pageSettings }
    }
    if (editor.value && res.document?.htmlContent) {
      editor.value.commands.setContent(res.document.htmlContent, { emitUpdate: false })
    }
    await nextTick()
    recomputePageGuides()
  } catch (e: any) {
    uiStore.notify(`Failed to load document editor: ${e?.message ?? 'Unknown error'}`, 'error')
  } finally {
    loadingDoc.value = false
  }
}

function buildPayload(isDraft: boolean): EditedDocument {
  return {
    month: `${props.year}-${props.month}`,
    htmlContent: editor.value?.getHTML() || '',
    pageSettings: { ...pageSettings.value },
    isDraft,
    updatedAt: new Date().toISOString(),
    sourceMode: 'editor',
  }
}

async function handleSaveDraft() {
  if (!editor.value) return
  savingDraft.value = true
  try {
    const st = await editorApi.saveDraft(props.year, props.month, buildPayload(true))
    status.value = st
    emit('status-change', st)
    uiStore.notify('Editing draft saved locally.', 'success')
  } catch (e: any) {
    uiStore.notify(`Failed to save draft: ${e?.message ?? 'Unknown error'}`, 'error')
  } finally {
    savingDraft.value = false
  }
}

async function handleSaveChanges() {
  if (!editor.value) return
  savingFinal.value = true
  try {
    const st = await editorApi.saveChanges(props.year, props.month, buildPayload(false))
    status.value = st
    emit('status-change', st)
    emit('saved', st)
    uiStore.notify('Saved changes, updated DOCX & refreshed PDF preview!', 'success')
  } catch (e: any) {
    uiStore.notify(`Failed to save changes: ${e?.message ?? 'Unknown error'}`, 'error')
  } finally {
    savingFinal.value = false
  }
}

async function handleDiscardChanges() {
  if (!window.confirm('Discard unsaved draft changes and restore the most recent saved version?')) {
    return
  }
  try {
    const res = await editorApi.discard(props.year, props.month)
    status.value = res.status
    emit('status-change', res.status)
    if (res.document?.pageSettings) {
      pageSettings.value = { ...pageSettings.value, ...res.document.pageSettings }
    }
    if (editor.value && res.document?.htmlContent) {
      editor.value.commands.setContent(res.document.htmlContent, { emitUpdate: false })
    }
    await nextTick()
    recomputePageGuides()
    uiStore.notify('Restored most recent saved version.', 'info')
  } catch (e: any) {
    uiStore.notify(`Failed to discard changes: ${e?.message ?? 'Unknown error'}`, 'error')
  }
}

async function handleOpenInWord() {
  openingWord.value = true
  try {
    const res = await editorApi.openInWord(props.year, props.month)
    wordLaunchedPath.value = res.path
    uiStore.notify('Opened working copy in Microsoft Word. Save in Word, then click "Sync from Word".', 'info')
  } catch (e: any) {
    uiStore.notify(`Could not open Microsoft Word: ${e?.message ?? 'Unknown error'}`, 'error')
  } finally {
    openingWord.value = false
  }
}

async function handleSyncFromWord() {
  syncingWord.value = true
  try {
    const st = await editorApi.syncFromWord(props.year, props.month)
    status.value = st
    emit('status-change', st)
    emit('saved', st)
    uiStore.notify('Synced Microsoft Word edits into PDF preview & downloads!', 'success')
  } catch (e: any) {
    uiStore.notify(`Sync failed: ${e?.message ?? 'Unknown error'}`, 'error')
  } finally {
    syncingWord.value = false
  }
}

// ─── Formatting Helpers ──────────────────────────────────────────────────────

function applyHeadingStyle(level: string) {
  if (!editor.value) return
  if (level === 'p') {
    editor.value.chain().focus().setParagraph().run()
  } else {
    const lvl = Number(level) as 1 | 2 | 3
    editor.value.chain().focus().toggleHeading({ level: lvl }).run()
  }
}

function applyFontFamily(font: string) {
  selectedFont.value = font
  editor.value?.chain().focus().setFontFamily(font).run()
}

function applyFontSize(pt: string) {
  selectedFontSize.value = pt
  ;(editor.value?.chain().focus() as any).setFontSize(pt).run()
}

function applyColor(hex: string) {
  selectedColor.value = hex
  editor.value?.chain().focus().setColor(hex).run()
}

function applyLineHeight(lh: string) {
  selectedLineHeight.value = lh
  if (!editor.value) return
  editor.value.chain().focus().updateAttributes('paragraph', { lineHeight: lh }).updateAttributes('heading', { lineHeight: lh }).run()
  schedulePageMeasure()
}

function adjustParagraphSpacing(deltaPt: number) {
  if (!editor.value) return
  const curAttrs = editor.value.getAttributes('paragraph')
  const cur = parseFloat(curAttrs.marginBottom || '8pt') || 8
  const next = Math.max(0, Math.min(36, cur + deltaPt))
  editor.value.chain().focus().updateAttributes('paragraph', { marginBottom: `${next}pt` }).run()
  schedulePageMeasure()
}

function adjustIndent(deltaPx: number) {
  if (!editor.value) return
  const curAttrs = editor.value.getAttributes('paragraph')
  const cur = Number(curAttrs.indentPx || 0)
  const next = Math.max(0, Math.min(180, cur + deltaPx))
  editor.value.chain().focus().updateAttributes('paragraph', { indentPx: next }).run()
}

function insertPageBreak() {
  ;(editor.value?.chain().focus() as any).insertPageBreak().run()
  schedulePageMeasure()
}

function removeSelectedOrNearestPageBreak() {
  if (!editor.value) return
  if (editor.value.isActive('pageBreak')) {
    editor.value.chain().focus().deleteSelection().run()
    schedulePageMeasure()
    return
  }
  // Remove the last pageBreak node if none is actively selected
  const { state, dispatch } = editor.value.view
  let lastPos = -1
  state.doc.descendants((node, pos) => {
    if (node.type.name === 'pageBreak') lastPos = pos
  })
  if (lastPos >= 0) {
    dispatch(state.tr.delete(lastPos, lastPos + 1))
    schedulePageMeasure()
    uiStore.notify('Removed manual page break.', 'info')
  } else {
    uiStore.notify('No manual page breaks in document.', 'info')
  }
}

function triggerImageUpload() {
  imageInputRef.value?.click()
}

function handleImageSelected(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file || !editor.value) return
  const reader = new FileReader()
  reader.onload = () => {
    const src = reader.result as string
    editor.value?.chain().focus().setImage({ src }).run()
    schedulePageMeasure()
  }
  reader.readAsDataURL(file)
  input.value = ''
}

function resizeSelectedImage(widthPx: string) {
  editor.value?.chain().focus().updateAttributes('image', { width: widthPx }).run()
  schedulePageMeasure()
}

function alignSelectedImage(align: 'left' | 'center' | 'right') {
  editor.value?.chain().focus().updateAttributes('image', { align }).run()
}

async function execClipboard(cmd: 'cut' | 'copy' | 'paste') {
  editor.value?.chain().focus().run()
  if (cmd === 'paste' && navigator.clipboard?.readText) {
    try {
      const txt = await navigator.clipboard.readText()
      if (txt) editor.value?.chain().focus().insertContent(txt).run()
      return
    } catch {
      // Fallback below
    }
  }
  document.execCommand(cmd)
}

watch(() => [props.year, props.month], loadDocument)
watch(pageSettings, schedulePageMeasure, { deep: true })

onMounted(loadDocument)
onBeforeUnmount(() => {
  if (measureTimer) clearTimeout(measureTimer)
  editor.value?.destroy()
})
</script>

<template>
  <div class="flex flex-col gap-3">
    <!-- ── Top Action Bar ── -->
    <div class="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 px-4 py-2.5 flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-2 flex-wrap">
        <span class="text-xs font-semibold px-2.5 py-1 rounded-full bg-primary-50 dark:bg-primary-900/30 text-primary-700 dark:text-primary-300">
          Word-Like Document Editor • {{ estimatedPages }} {{ estimatedPages === 1 ? 'Page' : 'Pages' }}
        </span>
        <span v-if="status.hasDraft" class="text-xs px-2 py-0.5 rounded bg-amber-100 text-amber-800 font-medium">
          Unsaved Draft
        </span>
        <span v-else-if="status.hasSavedEdits" class="text-xs px-2 py-0.5 rounded bg-green-100 text-green-800 font-medium">
          Saved Manual Edits Active
        </span>
      </div>

      <div class="flex items-center gap-2 flex-wrap">
        <button
          class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-blue-300 dark:border-blue-700 text-blue-700 dark:text-blue-300 text-xs font-medium hover:bg-blue-50 dark:hover:bg-blue-900/20"
          :disabled="openingWord"
          title="Opens a working copy in locally installed Microsoft Word without modifying any master template"
          @click="handleOpenInWord"
        >
          <FileText class="w-3.5 h-3.5" />
          <span>{{ openingWord ? 'Opening…' : 'Open in Microsoft Word' }}</span>
        </button>

        <button
          v-if="wordLaunchedPath"
          class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-indigo-300 text-indigo-700 text-xs font-medium hover:bg-indigo-50"
          :disabled="syncingWord"
          @click="handleSyncFromWord"
        >
          <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': syncingWord }" />
          <span>{{ syncingWord ? 'Syncing…' : 'Sync from Word' }}</span>
        </button>

        <button
          class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-gray-300 dark:border-gray-700 text-gray-700 dark:text-gray-200 text-xs font-medium hover:bg-gray-50 dark:hover:bg-gray-800"
          :disabled="savingDraft"
          @click="handleSaveDraft"
        >
          <Save class="w-3.5 h-3.5" />
          <span>{{ savingDraft ? 'Saving Draft…' : 'Save Draft' }}</span>
        </button>

        <button
          class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-red-200 text-red-600 text-xs font-medium hover:bg-red-50"
          @click="handleDiscardChanges"
        >
          <RotateCcw class="w-3.5 h-3.5" />
          <span>Discard Changes</span>
        </button>

        <button
          class="flex items-center gap-1.5 px-4 py-1.5 rounded-lg bg-primary-600 hover:bg-primary-700 text-white text-xs font-semibold shadow-sm disabled:opacity-50"
          :disabled="savingFinal"
          @click="handleSaveChanges"
        >
          <Check class="w-3.5 h-3.5" />
          <span>{{ savingFinal ? 'Saving & Rendering PDF…' : 'Save Changes' }}</span>
        </button>
      </div>
    </div>

    <!-- ── Word-Like Formatting Ribbon ── -->
    <div v-if="editor" class="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-2.5 flex flex-wrap items-center gap-1.5 text-xs">
      <!-- Undo / Redo / Clipboard -->
      <button class="p-1.5 rounded hover:bg-gray-100 dark:hover:bg-gray-800" title="Undo" @click="editor.chain().focus().undo().run()"><Undo2 class="w-4 h-4" /></button>
      <button class="p-1.5 rounded hover:bg-gray-100 dark:hover:bg-gray-800" title="Redo" @click="editor.chain().focus().redo().run()"><Redo2 class="w-4 h-4" /></button>
      <button class="p-1.5 rounded hover:bg-gray-100 dark:hover:bg-gray-800" title="Cut" @click="execClipboard('cut')"><Scissors class="w-4 h-4" /></button>
      <button class="p-1.5 rounded hover:bg-gray-100 dark:hover:bg-gray-800" title="Copy" @click="execClipboard('copy')"><Copy class="w-4 h-4" /></button>
      <button class="p-1.5 rounded hover:bg-gray-100 dark:hover:bg-gray-800" title="Paste" @click="execClipboard('paste')"><ClipboardPaste class="w-4 h-4" /></button>

      <div class="h-5 w-px bg-gray-200 dark:bg-gray-700 mx-1" />

      <!-- Heading Style -->
      <select
        class="px-2 py-1 rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 text-xs"
        @change="applyHeadingStyle(($event.target as HTMLSelectElement).value)"
      >
        <option value="p">Normal</option>
        <option value="1">Project Heading (H1)</option>
        <option value="2">Task Title (H2)</option>
        <option value="3">Subheading (H3)</option>
      </select>

      <!-- Font Family -->
      <select
        v-model="selectedFont"
        class="px-2 py-1 rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 text-xs"
        @change="applyFontFamily(selectedFont)"
      >
        <option value="Times New Roman">Times New Roman</option>
        <option value="Century Gothic">Century Gothic</option>
        <option value="Calibri">Calibri</option>
        <option value="Arial">Arial</option>
        <option value="Georgia">Georgia</option>
        <option value="Avenir Next LT Pro Light">Avenir Next</option>
      </select>

      <!-- Font Size -->
      <select
        v-model="selectedFontSize"
        class="px-2 py-1 rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 text-xs"
        @change="applyFontSize(selectedFontSize)"
      >
        <option v-for="pt in ['10pt','11pt','12pt','14pt','15pt','16pt','18pt','20pt','23pt','26pt','28pt']" :key="pt" :value="pt">{{ pt }}</option>
      </select>

      <div class="h-5 w-px bg-gray-200 dark:bg-gray-700 mx-1" />

      <!-- Basic Marks + Color -->
      <button :class="['p-1.5 rounded', editor.isActive('bold') ? 'bg-primary-100 text-primary-700' : 'hover:bg-gray-100']" title="Bold" @click="editor.chain().focus().toggleBold().run()"><Bold class="w-4 h-4" /></button>
      <button :class="['p-1.5 rounded', editor.isActive('italic') ? 'bg-primary-100 text-primary-700' : 'hover:bg-gray-100']" title="Italic" @click="editor.chain().focus().toggleItalic().run()"><Italic class="w-4 h-4" /></button>
      <button :class="['p-1.5 rounded', editor.isActive('underline') ? 'bg-primary-100 text-primary-700' : 'hover:bg-gray-100']" title="Underline" @click="editor.chain().focus().toggleUnderline().run()"><UnderlineIcon class="w-4 h-4" /></button>
      <input v-model="selectedColor" type="color" class="w-6 h-6 p-0 border-0 rounded cursor-pointer" title="Font Color" @input="applyColor(selectedColor)" />

      <div class="h-5 w-px bg-gray-200 dark:bg-gray-700 mx-1" />

      <!-- Alignment -->
      <button :class="['p-1.5 rounded', editor.isActive({ textAlign: 'left' }) ? 'bg-primary-100 text-primary-700' : 'hover:bg-gray-100']" title="Align Left" @click="editor.chain().focus().setTextAlign('left').run()"><AlignLeft class="w-4 h-4" /></button>
      <button :class="['p-1.5 rounded', editor.isActive({ textAlign: 'center' }) ? 'bg-primary-100 text-primary-700' : 'hover:bg-gray-100']" title="Align Center" @click="editor.chain().focus().setTextAlign('center').run()"><AlignCenter class="w-4 h-4" /></button>
      <button :class="['p-1.5 rounded', editor.isActive({ textAlign: 'right' }) ? 'bg-primary-100 text-primary-700' : 'hover:bg-gray-100']" title="Align Right" @click="editor.chain().focus().setTextAlign('right').run()"><AlignRight class="w-4 h-4" /></button>
      <button :class="['p-1.5 rounded', editor.isActive({ textAlign: 'justify' }) ? 'bg-primary-100 text-primary-700' : 'hover:bg-gray-100']" title="Justify" @click="editor.chain().focus().setTextAlign('justify').run()"><AlignJustify class="w-4 h-4" /></button>

      <div class="h-5 w-px bg-gray-200 dark:bg-gray-700 mx-1" />

      <!-- Line Spacing / Paragraph Spacing / Indent -->
      <select v-model="selectedLineHeight" class="px-1.5 py-1 rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 text-xs" title="Line Spacing" @change="applyLineHeight(selectedLineHeight)">
        <option value="1.0">1.0x</option>
        <option value="1.15">1.15x</option>
        <option value="1.35">1.35x</option>
        <option value="1.5">1.5x</option>
        <option value="2.0">2.0x</option>
      </select>
      <button class="px-1.5 py-1 rounded hover:bg-gray-100 border border-gray-200" title="Increase Paragraph Spacing" @click="adjustParagraphSpacing(4)">+Sp</button>
      <button class="px-1.5 py-1 rounded hover:bg-gray-100 border border-gray-200" title="Decrease Paragraph Spacing" @click="adjustParagraphSpacing(-4)">-Sp</button>
      <button class="p-1.5 rounded hover:bg-gray-100" title="Increase Indent" @click="adjustIndent(24)"><IndentIncrease class="w-4 h-4" /></button>
      <button class="p-1.5 rounded hover:bg-gray-100" title="Decrease Indent" @click="adjustIndent(-24)"><IndentDecrease class="w-4 h-4" /></button>

      <div class="h-5 w-px bg-gray-200 dark:bg-gray-700 mx-1" />

      <!-- Lists -->
      <button :class="['p-1.5 rounded', editor.isActive('bulletList') ? 'bg-primary-100 text-primary-700' : 'hover:bg-gray-100']" title="Bullet List" @click="editor.chain().focus().toggleBulletList().run()"><List class="w-4 h-4" /></button>
      <button :class="['p-1.5 rounded', editor.isActive('orderedList') ? 'bg-primary-100 text-primary-700' : 'hover:bg-gray-100']" title="Numbered List" @click="editor.chain().focus().toggleOrderedList().run()"><ListOrdered class="w-4 h-4" /></button>

      <div class="h-5 w-px bg-gray-200 dark:bg-gray-700 mx-1" />

      <!-- Page Breaks -->
      <button class="flex items-center gap-1 px-2 py-1 rounded border border-gray-200 hover:bg-gray-100" title="Insert Page Break" @click="insertPageBreak">
        <FilePlus2 class="w-3.5 h-3.5" />
        <span>Page Break</span>
      </button>
      <button class="flex items-center gap-1 px-2 py-1 rounded border border-gray-200 hover:bg-red-50 text-red-600" title="Remove Selected/Last Page Break" @click="removeSelectedOrNearestPageBreak">
        <Trash2 class="w-3.5 h-3.5" />
        <span>Remove Break</span>
      </button>

      <!-- Image & Table -->
      <input ref="imageInputRef" type="file" accept="image/*" class="hidden" @change="handleImageSelected" />
      <button class="flex items-center gap-1 px-2 py-1 rounded border border-gray-200 hover:bg-gray-100" title="Insert Image" @click="triggerImageUpload">
        <ImagePlus class="w-3.5 h-3.5" />
        <span>Image</span>
      </button>
      <button class="flex items-center gap-1 px-2 py-1 rounded border border-gray-200 hover:bg-gray-100" title="Insert 3x3 Table" @click="editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()">
        <TableIcon class="w-3.5 h-3.5" />
        <span>Table</span>
      </button>

      <!-- Page Setup Toggle -->
      <button :class="['flex items-center gap-1 px-2 py-1 rounded border ml-auto', showPageSetup ? 'bg-primary-50 border-primary-300 text-primary-700' : 'border-gray-200 hover:bg-gray-100']" @click="showPageSetup = !showPageSetup">
        <SlidersHorizontal class="w-3.5 h-3.5" />
        <span>Page Setup</span>
      </button>
    </div>

    <!-- Contextual Image / Table Controls -->
    <div v-if="editor && (editor.isActive('image') || editor.isActive('table'))" class="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg px-3 py-1.5 flex flex-wrap items-center gap-2 text-xs">
      <template v-if="editor.isActive('image')">
        <span class="font-semibold text-blue-800 dark:text-blue-200">Image:</span>
        <button class="px-2 py-0.5 rounded bg-white border hover:bg-gray-50" @click="resizeSelectedImage('200px')">Small</button>
        <button class="px-2 py-0.5 rounded bg-white border hover:bg-gray-50" @click="resizeSelectedImage('360px')">Medium</button>
        <button class="px-2 py-0.5 rounded bg-white border hover:bg-gray-50" @click="resizeSelectedImage('560px')">Full Width</button>
        <span class="text-gray-400">|</span>
        <button class="px-2 py-0.5 rounded bg-white border hover:bg-gray-50" @click="alignSelectedImage('left')">Left</button>
        <button class="px-2 py-0.5 rounded bg-white border hover:bg-gray-50" @click="alignSelectedImage('center')">Center</button>
        <button class="px-2 py-0.5 rounded bg-white border hover:bg-gray-50" @click="alignSelectedImage('right')">Right</button>
      </template>
      <template v-if="editor.isActive('table')">
        <span class="font-semibold text-blue-800 dark:text-blue-200">Table:</span>
        <button class="px-2 py-0.5 rounded bg-white border hover:bg-gray-50" @click="editor.chain().focus().addRowAfter().run()">+ Row</button>
        <button class="px-2 py-0.5 rounded bg-white border hover:bg-gray-50" @click="editor.chain().focus().deleteRow().run()">- Row</button>
        <button class="px-2 py-0.5 rounded bg-white border hover:bg-gray-50" @click="editor.chain().focus().addColumnAfter().run()">+ Col</button>
        <button class="px-2 py-0.5 rounded bg-white border hover:bg-gray-50" @click="editor.chain().focus().deleteColumn().run()">- Col</button>
        <button class="px-2 py-0.5 rounded bg-white border text-red-600 hover:bg-red-50" @click="editor.chain().focus().deleteTable().run()">Delete Table</button>
      </template>
    </div>

    <!-- Page Setup Panel (Margins, Page Size, Header/Footer) -->
    <div v-if="showPageSetup" class="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-3 grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-7 gap-2.5 text-xs">
      <div>
        <label class="block text-gray-500 mb-1">Page Size</label>
        <select v-model="pageSettings.pageSize" class="w-full px-2 py-1 rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800">
          <option value="Letter">US Letter (8.5×11 in)</option>
          <option value="A4">A4 (210×297 mm)</option>
        </select>
      </div>
      <div>
        <label class="block text-gray-500 mb-1">Top Margin (mm)</label>
        <input v-model.number="pageSettings.marginTopMM" type="number" step="1" class="w-full px-2 py-1 rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800" />
      </div>
      <div>
        <label class="block text-gray-500 mb-1">Bottom Margin (mm)</label>
        <input v-model.number="pageSettings.marginBottomMM" type="number" step="1" class="w-full px-2 py-1 rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800" />
      </div>
      <div>
        <label class="block text-gray-500 mb-1">Left Margin (mm)</label>
        <input v-model.number="pageSettings.marginLeftMM" type="number" step="1" class="w-full px-2 py-1 rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800" />
      </div>
      <div>
        <label class="block text-gray-500 mb-1">Right Margin (mm)</label>
        <input v-model.number="pageSettings.marginRightMM" type="number" step="1" class="w-full px-2 py-1 rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800" />
      </div>
      <div>
        <label class="block text-gray-500 mb-1">Header Text</label>
        <input v-model="pageSettings.headerText" type="text" placeholder="Optional header" class="w-full px-2 py-1 rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800" />
      </div>
      <div>
        <label class="block text-gray-500 mb-1">Footer Text</label>
        <input v-model="pageSettings.footerText" type="text" placeholder="Public" class="w-full px-2 py-1 rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800" />
      </div>
    </div>

    <!-- ── Paginated Document Canvas ── -->
    <div class="bg-gray-200 dark:bg-gray-950 rounded-xl border border-gray-300 dark:border-gray-800 p-6 overflow-auto" style="height: 68vh;">
      <div v-if="loadingDoc" class="flex items-center justify-center py-24 text-gray-500 text-sm">
        Loading document into Word-like editor…
      </div>
      <div
        v-else
        ref="editorHostRef"
        class="relative mx-auto bg-white text-gray-900 shadow-lg border border-gray-300"
        :style="{
          width: pageDimensionsPx.width + 'px',
          minHeight: pageDimensionsPx.height + 'px',
          paddingTop: pageDimensionsPx.padTop + 'px',
          paddingRight: pageDimensionsPx.padRight + 'px',
          paddingBottom: pageDimensionsPx.padBottom + 'px',
          paddingLeft: pageDimensionsPx.padLeft + 'px',
        }"
      >
        <!-- Header preview at top of page -->
        <div v-if="pageSettings.headerText" class="text-[10pt] text-gray-500 border-b border-gray-200 pb-1 mb-3 flex justify-between">
          <span>{{ pageSettings.headerText }}</span>
          <span>Page 1</span>
        </div>

        <!-- Visual Page Boundary Guides -->
        <div
          v-for="(y, idx) in pageBreakGuides"
          :key="idx"
          class="pointer-events-none absolute left-0 right-0 flex items-center justify-center"
          :style="{ top: y + 'px' }"
        >
          <div class="w-full border-t-2 border-dashed border-primary-400/70 relative">
            <span class="absolute right-3 -top-3 bg-primary-600 text-white text-[10px] font-semibold px-2 py-0.5 rounded shadow-sm">
              Page {{ idx + 2 }} Start
            </span>
          </div>
        </div>

        <EditorContent :editor="editor" class="word-editor-prose" />

        <!-- Footer preview at bottom -->
        <div class="text-[10pt] text-gray-500 border-t border-gray-200 pt-1 mt-6 flex justify-between">
          <span>{{ pageSettings.footerText || 'Public' }}</span>
          <span>{{ pageSettings.pageSize }} • {{ estimatedPages }} {{ estimatedPages === 1 ? 'page' : 'pages' }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.word-editor-prose :deep(.ProseMirror) {
  outline: none;
  min-height: 750px;
  font-family: 'Times New Roman', serif;
  font-size: 14pt;
  line-height: 1.35;
}
.word-editor-prose :deep(h1),
.word-editor-prose :deep(h2),
.word-editor-prose :deep(h3) {
  break-after: avoid;
  page-break-after: avoid;
}
.word-editor-prose :deep(ul) {
  list-style-type: disc;
  padding-left: 1.6em;
  margin: 0.35em 0;
}
.word-editor-prose :deep(ol) {
  list-style-type: decimal;
  padding-left: 1.6em;
  margin: 0.35em 0;
}
.word-editor-prose :deep(table) {
  width: 100%;
  border-collapse: collapse;
  margin: 0.75em 0;
}
.word-editor-prose :deep(td),
.word-editor-prose :deep(th) {
  border: 1px solid #cbd5e1;
  padding: 6px 8px;
  min-width: 60px;
}
.word-editor-prose :deep(.page-break-marker) {
  margin: 16px 0;
  border-top: 2px dashed #6366f1;
  text-align: center;
  position: relative;
  height: 12px;
  user-select: none;
}
.word-editor-prose :deep(.page-break-marker span) {
  background: #eef2ff;
  color: #4338ca;
  font-size: 11px;
  font-family: sans-serif;
  padding: 1px 8px;
  border-radius: 9999px;
  position: relative;
  top: -10px;
}
</style>
