import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Report } from '@/types'
import { reportsApi } from '@/api'

export const useReportsStore = defineStore('reports', () => {
  const reports = ref<Report[]>([])
  const currentReport = ref<Report | null>(null)
  const loading = ref(false)

  async function fetchReports() {
    loading.value = true
    try {
      reports.value = await reportsApi.list() || []
    } catch (e) {
      console.error('Failed to fetch reports', e)
    } finally {
      loading.value = false
    }
  }

  async function getReport(year: string, month: string) {
    loading.value = true
    try {
      currentReport.value = await reportsApi.get(year, month)
      return currentReport.value
    } catch (e) {
      currentReport.value = null
      return null
    } finally {
      loading.value = false
    }
  }

  async function generateReport(month: string, selectedTaskIds?: string[]) {
    loading.value = true
    try {
      const report = await reportsApi.generate(month, selectedTaskIds)
      currentReport.value = report
      // Update list
      const idx = reports.value.findIndex(r => r.month === month)
      if (idx !== -1) reports.value[idx] = report
      else reports.value.unshift(report)
      return report
    } finally {
      loading.value = false
    }
  }

  async function updateReport(year: string, month: string, report: Report) {
    loading.value = true
    try {
      const updated = await reportsApi.update(year, month, report)
      currentReport.value = updated
      return updated
    } finally {
      loading.value = false
    }
  }

  return {
    reports, currentReport, loading,
    fetchReports, getReport, generateReport, updateReport
  }
})
