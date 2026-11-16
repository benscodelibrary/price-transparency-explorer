import { createRouter, createWebHistory } from 'vue-router'
import SearchView from './views/SearchView.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'search', component: SearchView },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
  // Paging keeps the user's place at the top of the results, not the page.
  scrollBehavior: (_to, _from, saved) => saved ?? false,
})
