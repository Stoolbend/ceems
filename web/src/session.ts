import { ref } from 'vue'
import { ApiError, getSite, me, type User } from './api'

// The system name for the menu bar and tab title; CeeMS until loadSite succeeds.
export const systemName = ref('CeeMS')
export async function loadSite() {
  try {
    systemName.value = (await getSite()).systemName
  } catch {
    // keep the default
  }
}

// The logged-in user, or null when anonymous or not yet known.
export const user = ref<User | null>(null)

// ensureUser returns the cached user, else asks the server once and caches
// the answer. A 401 means anonymous (null); any other error is rethrown.
// Anonymous is not cached, so a later navigation asks again.
export async function ensureUser(): Promise<User | null> {
  if (user.value) return user.value
  try {
    user.value = await me()
  } catch (e) {
    if (!(e instanceof ApiError && e.status === 401)) throw e
  }
  return user.value
}
