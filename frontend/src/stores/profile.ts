import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Profile } from '@/types'
import { profileApi } from '@/api'

export const useProfileStore = defineStore('profile', () => {
  const profile = ref<Profile | null>(null)
  const loading = ref(false)
  const isFirstLaunch = ref(false)

  async function fetchProfile() {
    loading.value = true
    try {
      const p = await profileApi.get()
      if (!p || !p.employeeName) {
        isFirstLaunch.value = true
        profile.value = {
          employeeName: '',
          employeeCode: '',
          appraiserName: '',
          department: '',
          team: '',
          projects: [],
        }
      } else {
        profile.value = p
      }
    } catch (e) {
      console.error('Failed to fetch profile', e)
      isFirstLaunch.value = true
    } finally {
      loading.value = false
    }
  }

  async function saveProfile(p: Profile) {
    loading.value = true
    try {
      profile.value = await profileApi.update(p)
      isFirstLaunch.value = false
      return true
    } catch (e) {
      console.error('Failed to save profile', e)
      return false
    } finally {
      loading.value = false
    }
  }

  return { profile, loading, isFirstLaunch, fetchProfile, saveProfile }
})
