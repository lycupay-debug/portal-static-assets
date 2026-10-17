<script setup lang="ts">
import { ref } from 'vue'
const name = ref('')
const email = ref('')
const role = ref('rightsholder')
const workDescription = ref('')
const proofOfRights = ref('')
const infringingURLs = ref('')
const requestedAction = ref('review_and_remove')
const statementTruthful = ref(false)
const submitting = ref(false)
const error = ref('')
const result = ref<{ complaintId: string; message: string } | null>(null)
async function submit() {
  error.value = ''; result.value = null
  const urls = infringingURLs.value.split('\n').map(x => x.trim()).filter(Boolean)
  if (!name.value.trim() || !email.value.trim() || workDescription.value.trim().length < 5 || !urls.length || !statementTruthful.value) {
    error.value = '请填写必填信息、至少一个涉嫌侵权链接，并确认声明真实。'; return
  }
  submitting.value = true
  try {
    const response = await fetch('/api/public/complaints', { method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, credentials: 'same-origin', body: JSON.stringify({ name: name.value, email: email.value, role: role.value, workDescription: workDescription.value, proofOfRights: proofOfRights.value, infringingUrls: urls, requestedAction: requestedAction.value, statementTruthful: statementTruthful.value }) })
    const data = await response.json()
    if (!response.ok) throw new Error(data.error || '投诉提交失败。')
    result.value = data
    name.value = ''; email.value = ''; role.value = 'rightsholder'; workDescription.value = ''; proofOfRights.value = ''; infringingURLs.value = ''; requestedAction.value = 'review_and_remove'; statementTruthful.value = false
  } catch (e) { error.value = e instanceof Error ? e.message : '网络连接失败，请稍后重试。' }
  finally { submitting.value = false }
}
</script>
<template>
  <section class="copyright-complaint-form">
    <div class="ccf-heading"><p class="eyebrow">COPYRIGHT REPORT</p><h2>提交版权投诉</h2><p>请提供可核验的作品权利依据及具体页面链接。提交不代表投诉必然成立，平台会根据证据进行核验。</p></div>
    <p v-if="error" class="ccf-message error" role="alert">{{ error }}</p>
    <div v-if="result" class="ccf-message success" role="status"><strong>投诉已登记</strong><p>{{ result.message }}</p><p>投诉编号：<code>{{ result.complaintId }}</code></p><p>请保存该编号，以便后续联系时引用。</p></div>
    <form @submit.prevent="submit">
      <div class="ccf-grid">
        <label>投诉人姓名/名称 <span>*</span><input v-model="name" required maxlength="100" autocomplete="name"></label>
        <label>联系邮箱 <span>*</span><input v-model="email" required type="email" maxlength="254" autocomplete="email"></label>
        <label>投诉人身份<select v-model="role"><option value="rightsholder">权利人</option><option value="authorized_agent">授权代理人</option><option value="other">其他</option></select></label>
        <label>希望采取的措施<select v-model="requestedAction"><option value="review_and_remove">核验并视情况移除</option><option value="disable_access">限制访问</option><option value="credit_or_license">协商署名或授权</option><option value="other">其他</option></select></label>
        <label class="full">作品名称及权利说明 <span>*</span><textarea v-model="workDescription" required minlength="5" maxlength="3000" rows="4" placeholder="作品名称、作者/权利人、首次发布来源及涉嫌侵权的具体说明"></textarea></label>
        <label class="full">权利证明或代理授权材料说明<textarea v-model="proofOfRights" maxlength="5000" rows="4" placeholder="可填写登记证书、合同、原始发布链接、授权委托等证据的说明；请勿提交与核验无关的敏感信息"></textarea></label>
        <label class="full">涉嫌侵权页面链接（每行一个） <span>*</span><textarea v-model="infringingURLs" required rows="3" placeholder="https://www.lycupaynotion.top/content/example"></textarea></label>
      </div>
      <label class="ccf-confirm"><input v-model="statementTruthful" type="checkbox" required><span>我确认所提交的信息真实、准确，并有权提出本次投诉或已获得相应授权。</span></label>
      <button class="ccf-submit" type="submit" :disabled="submitting">{{ submitting ? '正在提交…' : '提交版权投诉' }}</button>
      <p class="ccf-privacy">投诉材料仅用于受理、核验和处理版权争议。请勿在表单中提交身份证号码、账户密码等非必要敏感信息。</p>
    </form>
  </section>
</template>
<style scoped>
.copyright-complaint-form{margin:44px 0 20px;padding-top:28px;border-top:1px solid #e7e7e7}.ccf-heading{margin-bottom:20px}.ccf-heading h2{font-size:23px;letter-spacing:-.025em;margin:0 0 9px}.ccf-heading p:not(.eyebrow){font-size:13px;color:#777;line-height:1.7}.ccf-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px}.ccf-grid label{display:flex;flex-direction:column;gap:7px;font-size:12px;color:#555}.ccf-grid label.full{grid-column:1/-1}.ccf-grid label span{color:#9a382b}.ccf-grid input,.ccf-grid select,.ccf-grid textarea{width:100%;min-width:0;border:1px solid #ddd;border-radius:6px;padding:10px;font:inherit;font-size:13px;color:#222;background:#fff}.ccf-grid textarea{resize:vertical;line-height:1.65}.ccf-confirm{display:flex;align-items:flex-start;gap:9px;margin:18px 0;font-size:12px;line-height:1.6;color:#555}.ccf-confirm input{margin-top:3px}.ccf-submit{background:#222;color:#fff;border:1px solid #222;border-radius:7px;padding:11px 16px;font-size:13px;cursor:pointer}.ccf-submit:disabled{opacity:.5;cursor:not-allowed}.ccf-privacy{font-size:11px;color:#888;line-height:1.6;margin-top:12px}.ccf-message{padding:12px;border-radius:7px;margin:14px 0;font-size:13px;line-height:1.6}.ccf-message.error{background:#fff0f0;color:#9a2828}.ccf-message.success{background:#f0f7f1;color:#275a2e}.ccf-message p{margin:6px 0}.ccf-message code{overflow-wrap:anywhere}@media(max-width:600px){.ccf-grid{grid-template-columns:1fr}.ccf-grid label.full{grid-column:auto}}
</style>
