// Bootstrap 6 official precompiled CSS; portal-specific styles remain later in the cascade.
import 'bootstrap/dist/css/bootstrap.min.css'
import './bootstrap-icons-local.css'
import { createApp } from 'vue'
import App from './App.vue'
import './monochrome.css'
import './notification.css'
import { initializeResponsiveLayout } from './utils/responsiveLayout'

initializeResponsiveLayout()
createApp(App).mount('#app')
