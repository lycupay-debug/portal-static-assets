import { createApp } from 'vue'
import App from './App.vue'
import { ElAutocomplete, ElDropdown, ElDropdownMenu, ElDropdownItem, ElIcon } from 'element-plus'
import 'element-plus/dist/index.css'
import { initializeResponsiveLayout } from './utils/responsiveLayout'

initializeResponsiveLayout()
createApp(App)
  .component('ElAutocomplete', ElAutocomplete)
  .component('ElDropdown', ElDropdown)
  .component('ElDropdownMenu', ElDropdownMenu)
  .component('ElDropdownItem', ElDropdownItem)
  .component('ElIcon', ElIcon)
  .mount('#app')
