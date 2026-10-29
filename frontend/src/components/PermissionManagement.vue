<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { apiRequest } from '../api/client'

type PortalUser = { id: string; email: string; displayName: string; isAdmin: boolean; isCopyrightReviewer: boolean; createdAt: string; roles: string[] }
type PortalRole = { roleKey: string; displayName: string; description: string; isSystem: boolean; permissions: string[] }
type PortalPermission = { permissionKey: string; description: string }

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const users = ref<PortalUser[]>([])
const roles = ref<PortalRole[]>([])
const permissions = ref<PortalPermission[]>([])
const selectedRole = ref('')
const selectedUser = ref('')
const selectedUserRoles = ref<string[]>([])
const selectedRolePermissions = ref<string[]>([])
const query = ref('')
const filteredUsers = computed(() => {
  const term = query.value.trim().toLocaleLowerCase()
  return users.value.filter(user => !term || user.email.toLocaleLowerCase().includes(term) || user.displayName.toLocaleLowerCase().includes(term))
})
const role = computed(() => roles.value.find(item => item.roleKey === selectedRole.value) || null)

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  return apiRequest<T>(path, { cache: 'no-store', ...init })
}
async function refresh() {
  loading.value = true
  error.value = ''
  try {
    const [userResult, roleResult, permissionResult] = await Promise.all([
      api<{ items: PortalUser[] }>('/admin/users'),
      api<{ items: PortalRole[] }>('/admin/roles'),
      api<{ items: PortalPermission[] }>('/admin/permissions'),
    ])
    users.value = userResult.items || []
    roles.value = roleResult.items || []
    permissions.value = permissionResult.items || []
    if (!selectedRole.value || !roles.value.some(item => item.roleKey === selectedRole.value)) selectedRole.value = roles.value[0]?.roleKey || ''
    if (!selectedUser.value || !users.value.some(item => item.id === selectedUser.value)) selectedUser.value = users.value.find(item => item.email === 'lycupay@gmail.com')?.id || users.value[0]?.id || ''
    syncSelections()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '读取权限数据失败'
  } finally {
    loading.value = false
  }
}
function syncSelections() {
  const user = users.value.find(item => item.id === selectedUser.value)
  selectedUserRoles.value = user ? [...user.roles] : []
  const currentRole = roles.value.find(item => item.roleKey === selectedRole.value)
  selectedRolePermissions.value = currentRole ? [...currentRole.permissions] : []
}
function chooseUser(id: string) { selectedUser.value = id; syncSelections(); notice.value = '' }
function chooseRole(key: string) { selectedRole.value = key; syncSelections(); notice.value = '' }
async function saveUserRoles() {
  const target = users.value.find(item => item.id === selectedUser.value)
  if (!target) return
  if (target.email === 'lycupay@gmail.com' && !selectedUserRoles.value.includes('super_admin')) {
    error.value = '指定超级管理员必须保留 super_admin 角色。'
    return
  }
  if (target.email !== 'lycupay@gmail.com' && selectedUserRoles.value.includes('super_admin')) {
    error.value = '系统策略只允许 lycupay@gmail.com 担任超级管理员。'
    return
  }
  saving.value = true; error.value = ''; notice.value = ''
  try {
    await api(`/admin/users/${encodeURIComponent(target.id)}/roles`, { method: 'PATCH', body: JSON.stringify({ roles: selectedUserRoles.value }) })
    notice.value = `${target.email} 的角色已更新。`
    await refresh()
  } catch (e) { error.value = e instanceof Error ? e.message : '保存角色失败' }
  finally { saving.value = false }
}
async function saveRolePermissions() {
  if (!selectedRole.value || selectedRole.value === 'super_admin') return
  saving.value = true; error.value = ''; notice.value = ''
  try {
    await api(`/admin/roles/${encodeURIComponent(selectedRole.value)}/permissions`, { method: 'PATCH', body: JSON.stringify({ permissions: selectedRolePermissions.value }) })
    notice.value = `角色“${role.value?.displayName || selectedRole.value}”的权限已更新。`
    await refresh()
  } catch (e) { error.value = e instanceof Error ? e.message : '保存权限失败' }
  finally { saving.value = false }
}
onMounted(refresh)
</script>

<template>
  <section class="permission-center">
    <div class="permission-heading">
      <div><p class="permission-eyebrow">ACCESS CONTROL</p><h1>权限管理</h1><p>管理账户角色与角色权限。所有变更均由服务器端校验并写入审计日志。</p></div>
      <button class="btn permission-secondary" type="button" :disabled="loading || saving" @click="refresh">刷新</button>
    </div>
    <p v-if="error" class="permission-alert" role="alert">{{ error }}</p>
    <p v-if="notice" class="permission-notice" role="status">{{ notice }}</p>
    <div v-if="loading" class="permission-empty">正在读取权限配置…</div>
    <div v-else class="permission-grid">
      <section class="permission-panel">
        <header><h2>账户角色</h2><span>{{ users.length }} 个账户</span></header>
        <input v-model="query" class="form-control permission-search" type="search" placeholder="搜索邮箱或昵称" aria-label="搜索账户" />
        <div class="permission-user-list">
          <button v-for="item in filteredUsers" :key="item.id" type="button" class="permission-user" :disabled="saving" :class="{ active: item.id === selectedUser }" @click="chooseUser(item.id)">
            <span class="permission-user-name">{{ item.displayName || '未设置昵称' }}</span><span class="permission-user-email">{{ item.email }}</span>
            <span class="permission-tags"><i v-for="key in item.roles" :key="key">{{ roles.find(r => r.roleKey === key)?.displayName || key }}</i></span>
          </button>
          <p v-if="!filteredUsers.length" class="permission-empty">没有匹配账户</p>
        </div>
        <div v-if="users.find(item => item.id === selectedUser)" class="permission-editor">
          <h3>分配角色</h3>
          <label v-for="item in roles" :key="item.roleKey" class="permission-check">
            <input v-model="selectedUserRoles" type="checkbox" :value="item.roleKey" :disabled="item.roleKey === 'super_admin' || saving" />
            <span><strong>{{ item.displayName }}</strong><small>{{ item.description }}</small></span>
          </label>
          <p class="permission-footnote">超级管理员只能由指定账户担任，不能通过此界面转授。</p>
          <button class="btn permission-primary" type="button" :disabled="saving" @click="saveUserRoles">{{ saving ? '正在保存…' : '保存账户角色' }}</button>
        </div>
      </section>
      <section class="permission-panel">
        <header><h2>角色权限</h2><span>{{ permissions.length }} 项权限</span></header>
        <label class="permission-field-label" for="permission-role">选择角色</label>
        <select id="permission-role" v-model="selectedRole" class="form-select" :disabled="saving" @change="syncSelections">
          <option v-for="item in roles" :key="item.roleKey" :value="item.roleKey">{{ item.displayName }}</option>
        </select>
        <div v-if="role" class="permission-editor">
          <p class="permission-role-description">{{ role.description }}</p>
          <label v-for="item in permissions" :key="item.permissionKey" class="permission-check">
            <input v-model="selectedRolePermissions" type="checkbox" :value="item.permissionKey" :disabled="selectedRole === 'super_admin' || saving" />
            <span><strong>{{ item.permissionKey }}</strong><small>{{ item.description }}</small></span>
          </label>
          <p v-if="selectedRole === 'super_admin'" class="permission-footnote">超级管理员保留系统完整权限，基础权限不可通过界面移除。</p>
          <button v-else class="btn permission-primary" type="button" :disabled="saving" @click="saveRolePermissions">{{ saving ? '正在保存…' : '保存角色权限' }}</button>
        </div>
      </section>
    </div>
  </section>
</template>

<style scoped>
.permission-center{display:grid;gap:18px;color:var(--ink,#242424)}.permission-heading{display:flex;justify-content:space-between;align-items:flex-start;gap:20px}.permission-heading h1{margin:2px 0 8px;font-size:clamp(28px,3vw,38px);letter-spacing:-.04em}.permission-heading p:not(.permission-eyebrow){margin:0;color:var(--muted,#777);font-size:14px;line-height:1.65}.permission-eyebrow{margin:0;color:#888;font-size:11px;letter-spacing:.14em;font-weight:700}.permission-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.permission-panel{min-width:0;padding:18px;border:1px solid rgba(120,120,120,.16);border-radius:18px;background:rgba(255,255,255,.32)}.permission-panel>header{display:flex;justify-content:space-between;align-items:center;gap:12px;margin-bottom:14px}.permission-panel h2{margin:0;font-size:18px}.permission-panel header span{font-size:12px;color:#888}.permission-search{margin-bottom:10px}.permission-user-list{display:grid;gap:7px;max-height:330px;overflow:auto}.permission-user{display:grid;text-align:left;gap:4px;width:100%;padding:11px 12px;border:1px solid transparent;border-radius:12px;background:rgba(120,120,120,.055);color:inherit}.permission-user.active{border-color:rgba(80,80,80,.24);background:rgba(120,120,120,.12)}.permission-user-name{font-size:14px;font-weight:650}.permission-user-email{font-size:12px;color:#858585;overflow-wrap:anywhere}.permission-tags{display:flex;flex-wrap:wrap;gap:5px}.permission-tags i{font-style:normal;font-size:11px;color:#555;background:rgba(100,100,100,.1);padding:3px 6px;border-radius:5px}.permission-editor{display:grid;gap:11px;margin-top:18px;padding-top:16px;border-top:1px solid rgba(120,120,120,.15)}.permission-editor h3{margin:0;font-size:15px}.permission-check{display:flex;align-items:flex-start;gap:10px;padding:9px 0}.permission-check input{margin-top:4px;accent-color:#555}.permission-check span{display:grid;gap:4px;min-width:0}.permission-check strong{font-size:13px;overflow-wrap:anywhere}.permission-check small{font-size:12px;line-height:1.5;color:#888}.permission-field-label{display:block;margin:2px 0 7px;color:#777;font-size:13px}.permission-role-description{margin:0;color:#777;font-size:13px;line-height:1.6}.permission-primary,.permission-secondary{border-radius:10px;padding:9px 13px;font-size:13px}.permission-primary{background:#292929;color:#fff}.permission-primary:disabled,.permission-secondary:disabled{opacity:.55}.permission-secondary{border:1px solid rgba(100,100,100,.25);color:inherit}.permission-footnote{margin:0;color:#888;font-size:12px;line-height:1.55}.permission-alert,.permission-notice{padding:11px 13px;border-radius:10px;font-size:13px;margin:0}.permission-alert{color:#8a2929;background:rgba(180,40,40,.08)}.permission-notice{color:#3e6748;background:rgba(40,130,70,.08)}.permission-empty{padding:20px 0;color:#888;font-size:13px;text-align:center}@media(max-width:900px){.permission-grid{grid-template-columns:minmax(0,1fr)}}@media(max-width:600px){.permission-panel{padding:13px}.permission-heading{align-items:flex-start}.permission-heading h1{font-size:28px}}
</style>
