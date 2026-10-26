import { createRouter, createWebHistory, type NavigationGuard, type Router,type RouteRecordRaw } from 'vue-router'
import PagesView from './PagesView.vue'
import PageView from './PageView.vue'
import LoginView from './LoginView.vue'
import TemplatesView from './TemplatesView.vue'
import BindingsView from './BindingsView.vue'
import FastextView from './FastextView.vue'
import UsersView from './UsersView.vue'
import KeysView from './KeysView.vue'
import PasswordView from './PasswordView.vue'
import SettingsView from './SettingsView.vue'
import HelpView from './HelpView.vue'
import { onUnauthorized } from './api'
import { ensureUser, user } from './session'

export const routes: RouteRecordRaw[] = [
  { path: '/', component: PagesView },
  { path: '/pages/:mpp', component: PageView, meta: { wide: true } },
  { path: '/templates', component: TemplatesView },
  { path: '/bindings', component: BindingsView },
  { path: '/fastext', component: FastextView },
  { path: '/users', component: UsersView },
  { path: '/keys', component: KeysView },
  { path: '/settings', component: SettingsView },
  { path: '/password', component: PasswordView },
  { path: '/help', component: HelpView },
  { path: '/login', component: LoginView, meta: { public: true } },
]

if (import.meta.env.DEV) {
  routes.push({ path: '/dev/gallery', component: () => import('./teletext/GalleryView.vue') })
}

// authGuard sends an anonymous visitor of any non-public route to /login.
// A network error is rethrown, failing the navigation.
export const authGuard: NavigationGuard = async (to) => {
  if (to.meta.public) return true
  if (await ensureUser()) return true
  return { path: '/login', query: { redirect: to.fullPath } }
}

export const router = createRouter({ history: createWebHistory(), routes })
router.beforeEach(authGuard)

// The session ended on the server: forget the user and go to login, back to here afterwards.
// The public check stops a second 401 from stacking redirects on /login.
export function expireSession(r: Router) {
  user.value = null
  const at = r.currentRoute.value
  if (!at.meta.public) void r.push({ path: '/login', query: { redirect: at.fullPath } })
}
onUnauthorized(() => expireSession(router))
