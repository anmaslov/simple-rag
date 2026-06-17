import { createRouter, createWebHistory } from 'vue-router'
import ChatView from '../views/ChatView.vue'
import SearchView from '../views/SearchView.vue'
import PagesView from '../views/PagesView.vue'
import SyncView from '../views/SyncView.vue'
import SettingsView from '../views/SettingsView.vue'

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/chat' },
    { path: '/chat', component: ChatView },
    { path: '/search', component: SearchView },
    { path: '/pages', component: PagesView },
    { path: '/sync', component: SyncView },
    { path: '/settings', component: SettingsView }
  ]
})
