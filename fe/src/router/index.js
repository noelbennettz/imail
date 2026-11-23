import { createRouter, createWebHashHistory } from 'vue-router'

// Routes are loaded lazily: the editor pulls in wangEditor and admin pulls in
// six settings panels, none of which should be part of the first paint.
const router = createRouter({
  history: createWebHashHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'home',
      component: () => import('../views/ListView.vue')
    },
    {
      path: '/list',
      name: 'list',
      component: () => import('../views/ListView.vue')
    },
    {
      path: '/editer',
      name: 'editer',
      component: () => import('../views/EditerView.vue')
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/LoginView.vue')
    },
    {
      path: '/setup',
      name: 'setup',
      component: () => import('../views/SetupView.vue')
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('../views/AdminView.vue')
    },
    {
      path: '/admin',
      redirect: '/settings'
    },
    {
      path: '/detail/:id',
      name: 'detail',
      component: () => import('../views/EmailDetailView.vue')
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('../views/NotFoundView.vue')
    }
  ]
})

// Every page except the login and setup screens needs a session. The check
// waits for the first /api/user/info answer, then lets the navigation continue.
router.beforeEach(async (to) => {
  if (to.name === 'login' || to.name === 'setup') return true
  const {useGlobalStatusStore} = await import('@/stores/useGlobalStatusStore')
  const store = useGlobalStatusStore()
  if (!store.isLogin) {
    await store.init()
  }
  if (!store.isLogin) {
    return {name: 'login', query: {redirect: to.fullPath}}
  }
  return true
})

export {router};
