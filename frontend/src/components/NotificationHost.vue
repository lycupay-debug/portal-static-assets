<script setup lang="ts">
import { dismissNotificationById, notificationState } from '../utils/notification'
const icons = { success: '✓', error: '×', warning: '!', info: 'i' } as const
</script>
<template>
  <Teleport to="body">
  <div class="portal-notification-stack" aria-label="系统通知" aria-live="polite" aria-relevant="additions">
    <TransitionGroup name="portal-notification">
      <section v-for="item in notificationState" :key="item.id" class="portal-notification" :class="`is-${item.type}`" role="status">
        <span class="portal-notification-icon" aria-hidden="true">{{ icons[item.type] }}</span>
        <div class="portal-notification-copy"><strong>{{ item.title }}</strong><p>{{ item.message }}</p></div>
        <button type="button" class="portal-notification-close" aria-label="关闭通知" @click="dismissNotificationById(item.id)">×</button>
      </section>
    </TransitionGroup>
  </div>
  </Teleport>
</template>
<style scoped>
.portal-notification-stack { position: fixed; z-index: 10000; top: 22px; right: 22px; display: flex; width: min(390px, calc(100vw - 32px)); flex-direction: column; gap: 10px; pointer-events: none; }
.portal-notification { display: flex; align-items: flex-start; gap: 12px; padding: 15px 14px; border: 1px solid rgba(20,24,32,.08); border-radius: 14px; background: rgba(255,255,255,.92); color: #202124; box-shadow: 0 14px 42px rgba(20,24,32,.13), 0 2px 8px rgba(20,24,32,.05); backdrop-filter: blur(24px) saturate(1.35); -webkit-backdrop-filter: blur(24px) saturate(1.35); pointer-events: auto; }
.portal-notification-icon { display: grid; width: 23px; height: 23px; flex: 0 0 23px; place-items: center; border-radius: 50%; font-size: 14px; font-weight: 700; }
.is-success .portal-notification-icon { color: #17764b; background: #e4f5eb; } .is-error .portal-notification-icon { color: #b4232c; background: #fce8e8; } .is-warning .portal-notification-icon { color: #946200; background: #fff2cc; } .is-info .portal-notification-icon { color: #315f9b; background: #e8f1ff; }
.portal-notification-copy { min-width: 0; flex: 1; } .portal-notification-copy strong { display: block; margin: 1px 0 4px; font-size: 13px; font-weight: 650; line-height: 1.4; } .portal-notification-copy p { margin: 0; color: #50545b; font-size: 13px; line-height: 1.6; overflow-wrap: anywhere; }
.portal-notification-close { width: 24px; height: 24px; margin: -3px -3px 0 0; border: 0; border-radius: 6px; background: transparent; color: #747880; cursor: pointer; font-size: 20px; line-height: 1; } .portal-notification-close:hover { background: rgba(20,24,32,.06); color: #202124; }
.portal-notification-enter-active, .portal-notification-leave-active, .portal-notification-move { transition: opacity .2s ease, transform .2s ease; } .portal-notification-enter-from, .portal-notification-leave-to { opacity: 0; transform: translate(12px,-5px); }
@media (max-width: 600px) { .portal-notification-stack { top: 12px; right: 12px; width: calc(100vw - 24px); } } @media (prefers-reduced-motion: reduce) { .portal-notification-enter-active, .portal-notification-leave-active, .portal-notification-move { transition: none; } }
</style>
