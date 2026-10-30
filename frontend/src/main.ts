// Bootstrap and portal styles are loaded before native views mount.
import 'bootstrap/dist/css/bootstrap.min.css'
import './bootstrap-icons-local.css'
import './app.css'
import './theme.css'
import './monochrome.css'
import './notification.css'
import { initializeResponsiveLayout } from './utils/responsiveLayout'
import { mountPortalApplication } from './portalApplication'

const root = document.querySelector<HTMLElement>('#app')
if (!root) throw new Error('Portal application root #app was not found')

const disposeResponsiveLayout = initializeResponsiveLayout()
let disposePortalApplication: (() => void) | undefined
try {
  disposePortalApplication = mountPortalApplication(root)
} catch (error) {
  disposeResponsiveLayout()
  throw error
}

const dispose = () => {
  try {
    disposePortalApplication?.()
  } finally {
    disposeResponsiveLayout()
  }
}

if (import.meta.hot) import.meta.hot.dispose(dispose)
