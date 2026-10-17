<script setup lang="ts">
import { ref, watch } from 'vue'

type RightsForm = {
  ownershipBasis: string
  rightsholder: string
  sourceUrl: string
  useScope: string
  licenseName: string
  licenseUrl: string
  permissionExpiry: string
  evidenceUri: string
  evidenceText: string
  evidenceFilename: string
  evidenceData: string
  statementAccepted: boolean
}
const props = defineProps<{ modelValue: RightsForm; compact?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: RightsForm] }>()
const local = ref<RightsForm>({ ...props.modelValue })
const evidenceFileError = ref('')
async function selectEvidenceFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  evidenceFileError.value = ''
  if (!file) { local.value.evidenceFilename = ''; local.value.evidenceData = ''; return }
  const allowed = /\\.(pdf|png|jpe?g|webp|txt|docx?)$/i.test(file.name)
  if (!allowed) { evidenceFileError.value = '证据文件仅支持 PDF、PNG、JPG、WEBP、TXT、DOC、DOCX。'; input.value = ''; return }
  if (file.size > 10 * 1024 * 1024) { evidenceFileError.value = '单个证据文件不能超过 10 MB。'; input.value = ''; return }
  try {
    const data = await new Promise<string>((resolve, reject) => { const reader = new FileReader(); reader.onerror = () => reject(new Error('读取证据文件失败。')); reader.onload = () => resolve(String(reader.result).split(',')[1] || ''); reader.readAsDataURL(file) })
    local.value.evidenceFilename = file.name
    local.value.evidenceData = data
  } catch (error) { evidenceFileError.value = error instanceof Error ? error.message : '读取证据文件失败。' }
}
watch(() => props.modelValue, value => { if (JSON.stringify(value) !== JSON.stringify(local.value)) local.value = { ...value } }, { deep: true })
watch(local, value => { if (JSON.stringify(value) !== JSON.stringify(props.modelValue)) emit('update:modelValue', { ...value }) }, { deep: true })
</script>

<template>
  <section class="publisher-rights-form">
    <h3>版权依据与证据材料</h3>
    <p>由发布者提供权利依据。提交后状态仍为待审核，不会自动获得发布资格。来源链接本身不等于传播授权。</p>
    <label><span>权利类型</span><select v-model="local.ownershipBasis" class="form-control"><option value="original">自主原创</option><option value="commissioned">委托创作</option><option value="licensed">获得授权</option><option value="third_party_permission">第三方许可</option><option value="open_license">开放许可</option><option value="public_domain">公共领域</option></select></label>
    <label><span>作者或权利人</span><input v-model="local.rightsholder" class="form-control" maxlength="500" placeholder="作者、出版社或授权主体（选填）" /></label>
    <label><span>原始来源链接</span><input v-model="local.sourceUrl" type="url" class="form-control" maxlength="2048" placeholder="https://…" /></label>
    <label><span>拟使用范围</span><select v-model="local.useScope" class="form-control"><option value="commercial_web">网站展示（含会员服务）</option><option value="internal_only">仅内部使用</option><option value="redistribution">允许再分发</option><option value="modification">允许修改</option><option value="multiple">多个使用范围</option></select></label>
    <label v-if="local.ownershipBasis === 'open_license'"><span>许可证名称/版本</span><input v-model="local.licenseName" class="form-control" maxlength="500" placeholder="例如：CC BY 4.0" /></label>
    <label v-if="local.ownershipBasis === 'open_license'"><span>许可证链接</span><input v-model="local.licenseUrl" type="url" class="form-control" maxlength="2048" placeholder="官方许可证页面" /></label>
    <label v-if="['licensed','third_party_permission','commissioned'].includes(local.ownershipBasis)"><span>授权有效期至</span><input v-model="local.permissionExpiry" type="date" class="form-control" /></label>
    <label><span>证据链接（选填）</span><input v-model="local.evidenceUri" type="url" class="form-control" maxlength="2048" placeholder="授权页面或可核验材料链接" /></label>
    <label><span>上传证据文件（选填）</span><input class="form-control" type="file" accept=".pdf,.png,.jpg,.jpeg,.webp,.txt,.doc,.docx" @change="selectEvidenceFile" /></label>
    <p v-if="local.evidenceFilename" class="publisher-evidence-file">已选择：{{ local.evidenceFilename }}</p>
    <p v-if="evidenceFileError" class="publisher-evidence-error">{{ evidenceFileError }}</p>
    <label><span>证据说明/授权条款</span><textarea v-model="local.evidenceText" class="form-control" maxlength="50000" rows="3" placeholder="说明原创情况、授权范围，或粘贴相关许可条款摘要"></textarea></label>
    <label class="resource-public-toggle"><input v-model="local.statementAccepted" type="checkbox" /><span><strong>我声明以上信息真实、完整，并且有权按所选范围使用该内容。</strong><small>虚假声明不会替代审核；材料提交后由版权审核人员独立核验。</small></span></label>
  </section>
</template>
