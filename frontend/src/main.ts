// Bootstrap 6 official precompiled CSS; portal-specific styles remain later in the cascade.
import 'bootstrap/dist/css/bootstrap.min.css'
import { createApp } from 'vue'
import App from './App.vue'
import { initializeResponsiveLayout } from './utils/responsiveLayout'

initializeResponsiveLayout()
createApp(App).mount('#app')
