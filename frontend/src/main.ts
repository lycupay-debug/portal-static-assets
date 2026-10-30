// Bootstrap and portal styles are loaded before native views mount.
import 'bootstrap/dist/css/bootstrap.min.css'
import './bootstrap-icons-local.css'
import './app.css'
import './theme.css'
import './monochrome.css'
import './notification.css'
import { initializeResponsiveLayout } from './utils/responsiveLayout'
import { mountPortalApplication } from './portalApplication'

initializeResponsiveLayout()
const root = document.querySelector<HTMLElement>('#app')
if (!root) throw new Error('Portal application root #app was not found')
const dispose = mountPortalApplication(root)

if (import.meta.hot) import.meta.hot.dispose(dispose)
