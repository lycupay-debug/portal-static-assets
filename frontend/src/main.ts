import { createApp } from 'vue'
import App from './App.vue'
import { initializeResponsiveLayout } from './utils/responsiveLayout'

initializeResponsiveLayout()
createApp(App).mount('#app')
