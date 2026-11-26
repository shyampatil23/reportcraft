import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useUIStore = defineStore('ui', () => {
  const sidebarOpen = ref(true)
  const notification = ref<{ message: string; type: 'success' | 'error' | 'info' } | null>(null)
  const confirmDialog = ref<{
    visible: boolean
    title: string
    message: string
    onConfirm: (() => void) | null
  }>({ visible: false, title: '', message: '', onConfirm: null })

  function notify(message: string, type: 'success' | 'error' | 'info' = 'success') {
    notification.value = { message, type }
    setTimeout(() => { notification.value = null }, 4000)
  }

  function confirm(title: string, message: string, onConfirm: () => void) {
    confirmDialog.value = { visible: true, title, message, onConfirm }
  }

  function closeConfirm() {
    confirmDialog.value = { visible: false, title: '', message: '', onConfirm: null }
  }

  function toggleSidebar() {
    sidebarOpen.value = !sidebarOpen.value
  }

  return { sidebarOpen, notification, confirmDialog, notify, confirm, closeConfirm, toggleSidebar }
})
