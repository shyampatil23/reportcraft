import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Task, TaskStats } from '@/types'
import { tasksApi } from '@/api'
import { format } from 'date-fns'

export const useTasksStore = defineStore('tasks', () => {
  const tasks = ref<Task[]>([])
  const stats = ref<TaskStats | null>(null)
  const loading = ref(false)
  const currentMonth = ref(format(new Date(), 'yyyy-MM'))

  const tasksByDate = computed(() => {
    const map: Record<string, Task[]> = {}
    for (const task of tasks.value) {
      if (!map[task.date]) map[task.date] = []
      map[task.date].push(task)
    }
    return map
  })

  const tasksByProject = computed(() => {
    const map: Record<string, Task[]> = {}
    for (const task of tasks.value) {
      if (!map[task.project]) map[task.project] = []
      map[task.project].push(task)
    }
    return map
  })

  async function fetchTasks(month?: string) {
    const m = month || currentMonth.value
    currentMonth.value = m
    loading.value = true
    try {
      const [year, mon] = m.split('-')
      tasks.value = await tasksApi.getMonth(year, mon) || []
    } catch (e) {
      console.error('Failed to fetch tasks', e)
      tasks.value = []
    } finally {
      loading.value = false
    }
  }

  async function fetchStats(month?: string) {
    const m = month || currentMonth.value
    try {
      stats.value = await tasksApi.getStats(m)
    } catch (e) {
      console.error('Failed to fetch stats', e)
    }
  }

  async function createTask(task: Partial<Task>) {
    loading.value = true
    try {
      const created = await tasksApi.create(task)
      tasks.value.push(created)
      tasks.value.sort((a, b) => a.date.localeCompare(b.date))
      return created
    } finally {
      loading.value = false
    }
  }

  async function updateTask(id: string, task: Partial<Task>) {
    loading.value = true
    try {
      const updated = await tasksApi.update(id, task)
      const idx = tasks.value.findIndex(t => t.id === id)
      if (idx !== -1) tasks.value[idx] = updated
      return updated
    } finally {
      loading.value = false
    }
  }

  async function deleteTask(id: string) {
    loading.value = true
    try {
      await tasksApi.delete(id, currentMonth.value)
      tasks.value = tasks.value.filter(t => t.id !== id)
    } finally {
      loading.value = false
    }
  }

  async function bulkImport(taskList: Task[]) {
    loading.value = true
    try {
      tasks.value = await tasksApi.bulkImport(currentMonth.value, taskList) || []
    } finally {
      loading.value = false
    }
  }

  return {
    tasks, stats, loading, currentMonth,
    tasksByDate, tasksByProject,
    fetchTasks, fetchStats, createTask, updateTask, deleteTask, bulkImport
  }
})
