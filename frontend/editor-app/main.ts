import 'bootstrap/dist/css/bootstrap.min.css'
import '../src/bootstrap-icons-local.css'
import '../src/monochrome.css'
import '../src/theme.css'
import { createApp } from 'vue'
import EditorApp from './EditorApp.vue'
import './editor.css'

createApp(EditorApp).mount('#editor-app')
