import { createApp } from 'vue'
import App from './App.vue'
import { ElAutocomplete } from 'element-plus'
import 'element-plus/dist/index.css'
import { initializeResponsiveLayout } from './utils/responsiveLayout'

initializeResponsiveLayout()
createApp(App).component('ElAutocomplete', ElAutocomplete).mount('#app')
