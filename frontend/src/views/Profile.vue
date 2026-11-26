<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  User, Check, Plus, X, Building, Briefcase,
  Users, Award, Save, RefreshCw, Layers
} from 'lucide-vue-next'
import { useProfileStore } from '@/stores/profile'
import { useUIStore } from '@/stores/ui'
import type { Profile } from '@/types'

const profileStore = useProfileStore()
const uiStore = useUIStore()

const isSaving = ref(false)
const newProjectInput = ref('')

const form = ref<Profile>({
  employeeName: '',
  employeeCode: '',
  appraiserName: '',
  department: '',
  team: '',
  projects: [],
})

function populateForm() {
  if (profileStore.profile) {
    form.value = {
      ...profileStore.profile,
      projects: [...(profileStore.profile.projects || [])],
    }
  }
}

function addProject() {
  const p = newProjectInput.value.trim()
  if (p && !form.value.projects.includes(p)) {
    form.value.projects.push(p)
    newProjectInput.value = ''
  }
}

function removeProject(index: number) {
  form.value.projects.splice(index, 1)
}

async function save() {
  if (!form.value.employeeName.trim()) {
    uiStore.notify('Employee Name is required', 'error')
    return
  }

  isSaving.value = true
  try {
    const ok = await profileStore.saveProfile(form.value)
    if (ok) {
      uiStore.notify('Profile updated successfully! Report cover pages will use this.')
    } else {
      uiStore.notify('Failed to save profile', 'error')
    }
  } catch (err: any) {
    uiStore.notify('Error saving: ' + err.message, 'error')
  } finally {
    isSaving.value = false
  }
}

const initials = computed(() => {
  const name = form.value.employeeName || ''
  return name.split(' ').map(w => w[0]).join('').toUpperCase().slice(0, 2) || 'ME'
})

onMounted(() => {
  populateForm()
})
</script>

<template>
  <div class="p-6 max-w-5xl mx-auto space-y-6">
    <!-- Header -->
    <div class="bg-white p-5 rounded-2xl shadow-sm border border-gray-100 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 flex items-center gap-2">
          <User class="w-6 h-6 text-primary-600" /> Employee Profile
        </h1>
        <p class="text-sm text-gray-500 mt-1">
          These details are automatically placed on your performance report cover pages and headers.
        </p>
      </div>

      <button @click="save" :disabled="isSaving" class="btn-primary text-xs flex items-center gap-2 shadow-sm font-semibold">
        <Save class="w-3.5 h-3.5" />
        {{ isSaving ? 'Saving...' : 'Save Profile Details' }}
      </button>
    </div>

    <!-- Main Profile Card -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      <!-- Left: Identity summary -->
      <div class="bg-white rounded-2xl p-6 shadow-sm border border-gray-100 text-center space-y-4">
        <div class="w-20 h-20 bg-gradient-to-br from-primary-600 to-violet-600 rounded-full flex items-center justify-center text-white text-2xl font-bold mx-auto shadow-md">
          {{ initials }}
        </div>

        <div>
          <h2 class="text-lg font-bold text-gray-900">{{ form.employeeName || 'Your Name' }}</h2>
          <p class="text-xs text-primary-600 font-semibold">{{ form.employeeCode || 'Employee Code' }}</p>
          <p class="text-xs text-gray-400 mt-1">{{ form.department || 'Department' }} • {{ form.team || 'Team' }}</p>
        </div>

        <div class="pt-4 border-t border-gray-100 text-left space-y-2 text-xs">
          <div class="flex items-center justify-between text-gray-600">
            <span>Appraiser:</span>
            <span class="font-semibold text-gray-900">{{ form.appraiserName || 'Not configured' }}</span>
          </div>
          <div class="flex items-center justify-between text-gray-600">
            <span>Active Projects:</span>
            <span class="font-semibold text-gray-900">{{ form.projects.length }}</span>
          </div>
        </div>
      </div>

      <!-- Right: Detailed edit form (2 cols) -->
      <div class="md:col-span-2 bg-white rounded-2xl p-6 shadow-sm border border-gray-100 space-y-5">
        <h3 class="text-sm font-bold uppercase tracking-wider text-gray-500 border-b border-gray-100 pb-2">
          Personal & Appraisal Information
        </h3>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="label">Employee Name <span class="text-red-500">*</span></label>
            <input
              v-model="form.employeeName"
              type="text"
              class="input-field"
              placeholder="e.g. John Doe"
            />
          </div>

          <div>
            <label class="label">Employee Code</label>
            <input
              v-model="form.employeeCode"
              type="text"
              class="input-field"
              placeholder="e.g. EMP1042"
            />
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="label">Appraiser / Manager Name</label>
            <input
              v-model="form.appraiserName"
              type="text"
              class="input-field"
              placeholder="e.g. Jane Smith"
            />
          </div>

          <div>
            <label class="label">Department</label>
            <input
              v-model="form.department"
              type="text"
              class="input-field"
              placeholder="e.g. Software Development"
            />
          </div>
        </div>

        <div>
          <label class="label">Team / Sub-unit</label>
          <input
            v-model="form.team"
            type="text"
            class="input-field"
            placeholder="e.g. Core Platform Engineering"
          />
        </div>

        <!-- Projects List with Tags -->
        <div class="pt-2">
          <label class="label">Assigned Projects</label>
          <p class="text-xs text-gray-400 mb-2">
            These project names will appear in dropdowns and on your report cover page.
          </p>

          <div class="flex flex-wrap gap-2 mb-3">
            <span
              v-for="(proj, idx) in form.projects"
              :key="proj"
              class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold bg-primary-50 text-primary-700 border border-primary-200"
            >
              {{ proj }}
              <button @click="removeProject(idx)" type="button" class="hover:text-red-600">
                <X class="w-3.5 h-3.5" />
              </button>
            </span>
          </div>

          <div class="flex gap-2">
            <input
              v-model="newProjectInput"
              @keydown.enter.prevent="addProject"
              type="text"
              class="input-field flex-1"
              placeholder="Add project (e.g. EMACloudContentManager, ERA 3.0)"
            />
            <button @click="addProject" type="button" class="btn-secondary text-xs flex items-center gap-1">
              <Plus class="w-3.5 h-3.5" /> Add Project
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
