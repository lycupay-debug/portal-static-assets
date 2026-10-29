/**
 * Responsibility: guard the permission editor's mutation/selection consistency contract.
 * Dependencies: Node test runner and component source boundary assertions.
 * Collaboration: prevents a pending role mutation from being relabeled by changing selection mid-request.
 * Boundaries: source-level checks only; no Vue mount or live privileged API mutation.
 */
import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/PermissionManagement.vue', import.meta.url), 'utf8')

test('account and role selection cannot change while a permission mutation is pending', () => {
  assert.match(source, /class="permission-user" :disabled="saving"/)
  assert.match(source, /id="permission-role" v-model="selectedRole" class="form-select" :disabled="saving"/)
  assert.match(source, /:disabled="saving" @click="saveUserRoles"/)
  assert.match(source, /:disabled="saving" @click="saveRolePermissions"/)
})

test('role assignment and permission mutation keep server-side API contracts', () => {
  assert.ok(source.includes('/admin/users/${encodeURIComponent(target.id)}/roles'))
  assert.ok(source.includes('/admin/roles/${encodeURIComponent(selectedRole.value)}/permissions'))
  assert.match(source, /method: 'PATCH'/)
  assert.match(source, /selectedRole\.value === 'super_admin'/)
  assert.match(source, /指定超级管理员必须保留 super_admin 角色/)
})
