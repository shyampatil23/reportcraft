import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  { path: '/', redirect: '/dashboard' },
  { path: '/dashboard', component: () => import('@/views/Dashboard.vue'), name: 'Dashboard' },
  { path: '/daily-tasks', component: () => import('@/views/DailyTasks.vue'), name: 'DailyTasks' },
  { path: '/monthly-tasks', component: () => import('@/views/MonthlyTaskEntry.vue'), name: 'MonthlyTaskEntry' },
  { path: '/full-month', component: () => import('@/views/FullMonthEntry.vue'), name: 'FullMonthEntry' },
  { path: '/my-tasks', component: () => import('@/views/MyTasks.vue'), name: 'MyTasks' },
  { path: '/generate-report', component: () => import('@/views/GenerateReport.vue'), name: 'GenerateReport' },
  { path: '/report-history', component: () => import('@/views/ReportHistory.vue'), name: 'ReportHistory' },
  { path: '/templates', component: () => import('@/views/Templates.vue'), name: 'Templates' },
  { path: '/profile', component: () => import('@/views/Profile.vue'), name: 'Profile' },
  { path: '/settings', component: () => import('@/views/Settings.vue'), name: 'Settings' },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
