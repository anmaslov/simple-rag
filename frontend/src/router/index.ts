import { createRouter, createWebHistory } from 'vue-router'
import ChatView from '../views/ChatView.vue'
import SearchView from '../views/SearchView.vue'
import PagesView from '../views/PagesView.vue'
import SettingsView from '../views/SettingsView.vue'
import SourcesView from '../views/SourcesView.vue'

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/chat' },
    { path: '/chat', component: ChatView },
    { path: '/search', component: SearchView },
    { path: '/pages', component: PagesView },
    { path: '/sources', component: SourcesView },
    { path: '/sync', redirect: '/sources' },
    { path: '/settings', component: SettingsView }
  ]
})
