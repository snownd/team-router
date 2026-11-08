import ArcoVue from '@arco-design/web-vue'
import ArcoVueIcon from '@arco-design/web-vue/es/icon'
import { createPinia } from 'pinia'
import { createApp } from 'vue'

import App from './App.vue'
import router from './router'

import '@arco-design/web-vue/dist/arco.css'
import './styles/theme.less'

createApp(App).use(createPinia()).use(router).use(ArcoVue).use(ArcoVueIcon).mount('#app')
