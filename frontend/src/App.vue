<script setup lang="ts">
import { computed, ref } from 'vue'

type PortalSection = {
  id: string
  number: string
  title: string
  subtitle: string
  description: string
  tags: string[]
}

const query = ref('')
const menuOpen = ref(false)

const sections: PortalSection[] = [
  {
    id: 'knowledge',
    number: '01',
    title: '知识文章',
    subtitle: 'KNOWLEDGE',
    description: '沉淀经过整理的思考、方法与实践记录，让知识可以被持续复用。',
    tags: ['思考与认知', '方法论', '实践复盘'],
  },
  {
    id: 'tools',
    number: '02',
    title: '在线工具',
    subtitle: 'TOOLS',
    description: '把重复步骤变成简单工具，减少操作成本，让精力回到真正重要的问题。',
    tags: ['效率工具', '文本处理', '数据辅助'],
  },
  {
    id: 'resources',
    number: '03',
    title: '资源资料',
    subtitle: 'RESOURCES',
    description: '按主题组织值得留存的资料、参考链接与学习入口。',
    tags: ['参考资料', '学习清单', '常用链接'],
  },
  {
    id: 'projects',
    number: '04',
    title: '项目展示',
    subtitle: 'PROJECTS',
    description: '记录正在构建的系统、已完成的实践，以及过程中的关键决策。',
    tags: ['项目记录', '技术实践', '构建日志'],
  },
]

const filteredSections = computed(() => {
  const keyword = query.value.trim().toLocaleLowerCase()
  if (!keyword) return sections
  return sections.filter((section) =>
    [section.title, section.subtitle, section.description, ...section.tags]
      .join(' ')
      .toLocaleLowerCase()
      .includes(keyword),
  )
})

function closeMenu() {
  menuOpen.value = false
}
</script>

<template>
  <div class="site-shell">
    <header class="site-header">
      <a class="brand" href="#top" aria-label="个人门户首页" @click="closeMenu">
        <span class="brand-mark">P.</span>
        <span class="brand-copy">
          <strong>PERSONAL PORTAL</strong>
          <small>知识 · 工具 · 实践</small>
        </span>
      </a>

      <button
        class="menu-toggle"
        type="button"
        :aria-expanded="menuOpen"
        aria-label="切换导航菜单"
        @click="menuOpen = !menuOpen"
      >
        <span></span><span></span>
      </button>

      <nav class="main-nav" :class="{ 'is-open': menuOpen }" aria-label="主导航">
        <a href="#explore" @click="closeMenu">探索</a>
        <a href="#about" @click="closeMenu">关于本站</a>
        <a class="nav-contact" href="#explore" @click="closeMenu">内容索引 <span>↘</span></a>
      </nav>
    </header>

    <main id="top">
      <section class="hero">
        <div class="hero-copy">
          <p class="eyebrow"><span class="status-dot"></span> A PERSONAL KNOWLEDGE SPACE</p>
          <h1>把想法整理好，<br /><span>让行动有迹可循。</span></h1>
          <p class="hero-description">
            一个持续生长的个人门户，用来整理知识、构建工具、归档资源，并记录从想法到实践的过程。
          </p>
          <div class="hero-actions">
            <a class="button button-primary" href="#explore">开始探索 <span>↘</span></a>
            <a class="text-link" href="#about">了解这个空间 <span>→</span></a>
          </div>
          <div class="hero-note">
            <span class="note-line"></span>
            <span>持续整理 · 持续构建 · 持续迭代</span>
          </div>
        </div>

        <div class="hero-art" aria-label="由层叠卡片构成的抽象知识结构插图" role="img">
          <div class="art-orbit orbit-one"></div>
          <div class="art-orbit orbit-two"></div>
          <div class="art-card card-back">
            <span class="art-card-label">FIELD NOTES</span>
            <span class="art-lines"><i></i><i></i><i></i></span>
          </div>
          <div class="art-card card-front">
            <span class="art-card-label">THOUGHT → ACTION</span>
            <div class="art-symbol">P<span>.</span></div>
            <div class="art-card-footer"><span>COLLECT</span><span>CONNECT</span><span>CREATE</span></div>
          </div>
          <span class="art-index">FIG. 001 — AN OPEN SYSTEM</span>
          <span class="art-spark spark-one">✳</span>
          <span class="art-spark spark-two">＋</span>
        </div>

        <div class="hero-bottom">
          <span>独立构建的个人空间</span>
          <span>01 / 04 <i></i> PUBLIC SECTIONS</span>
        </div>
      </section>

      <section id="explore" class="explore-section">
        <div class="section-heading">
          <div>
            <p class="eyebrow">EXPLORE THE PORTAL <span class="heading-rule"></span></p>
            <h2>从一个方向开始。</h2>
          </div>
          <p class="section-intro">四个入口，承载不同类型的内容。<br />结构先行，内容逐步积累。</p>
        </div>

        <label class="search-box">
          <span class="search-icon" aria-hidden="true">⌕</span>
          <input v-model="query" type="search" placeholder="搜索文章、工具、资源或项目分类…" aria-label="搜索门户分类" />
          <kbd>⌘ K</kbd>
        </label>

        <div v-if="filteredSections.length" class="section-grid">
          <article
            v-for="section in filteredSections"
            :key="section.id"
            class="section-card"
          >
            <div class="card-topline"><span>{{ section.number }}</span><span class="card-arrow">↗</span></div>
            <p class="card-kicker">{{ section.subtitle }}</p>
            <h3>{{ section.title }}</h3>
            <p class="card-description">{{ section.description }}</p>
            <div class="tag-list">
              <span v-for="tag in section.tags" :key="tag">{{ tag }}</span>
            </div>
            <div class="card-bottom"><span>内容逐步整理中</span><span>↗</span></div>
          </article>
        </div>
        <div v-else class="empty-search">
          <strong>没有匹配的分类</strong>
          <p>试试“知识”“工具”“资源”或“项目”。</p>
          <button type="button" @click="query = ''">清除搜索</button>
        </div>
      </section>

      <section id="about" class="about-section">
        <div class="about-mark">P.</div>
        <div class="about-copy">
          <p class="eyebrow">ABOUT THIS SPACE</p>
          <h2>不是内容的终点，<br />而是持续实践的起点。</h2>
          <p>这里将逐步收录公开的知识文章、在线工具、参考资料与项目记录。内容会在经过整理后发布；私人工作区与公开页面保持分离。</p>
        </div>
        <div class="about-meta">
          <span>DESIGNED TO GROW</span>
          <span>Built with intention.</span>
        </div>
      </section>
    </main>

    <footer class="site-footer">
      <a class="footer-brand" href="#top">PERSONAL PORTAL <span>©</span></a>
      <span>一个持续构建中的个人空间</span>
      <a href="#top">返回顶部 ↑</a>
    </footer>
  </div>
</template>

<style>
:root {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans SC', 'Microsoft YaHei', sans-serif;
  color: #1d2825;
  background: #f7f8f5;
  font-synthesis: none;
  text-rendering: optimizeLegibility;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  font-optical-sizing: auto;
  font-weight: 400;
  --ink: #1d2825;
  --muted: #69736e;
  --green: #276653;
  --line: #dfe4de;
  --paper: #f7f8f5;
  --mono: ui-monospace, SFMono-Regular, Consolas, monospace;
}

* { box-sizing: border-box; }
html { scroll-behavior: smooth; scroll-padding-top: 88px; }
body { margin: 0; min-width: 320px; }
a { color: inherit; text-decoration: none; }
button, input { font: inherit; }
button, a { -webkit-tap-highlight-color: transparent; }
button:focus-visible, a:focus-visible, input:focus-visible { outline: 2px solid #39856e; outline-offset: 4px; }
.site-shell { max-width: 1600px; margin: 0 auto; overflow: clip; }
.site-header { height: 82px; padding: 0 clamp(22px, 7.1vw, 112px); display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid var(--line); }
.brand { display: inline-flex; align-items: center; gap: 12px; }
.brand-mark { display: grid; place-items: center; width: 40px; height: 40px; background: var(--green); color: #fff; font-size: 21px; font-weight: 700; letter-spacing: -2px; border-radius: 11px 11px 11px 3px; }
.brand-copy { display: flex; flex-direction: column; gap: 3px; }
.brand-copy strong { font: 500 11px var(--mono); letter-spacing: .12em; }
.brand-copy small { color: var(--muted); font-size: 11px; letter-spacing: .08em; }
.main-nav { display: flex; align-items: center; gap: clamp(22px, 3.4vw, 52px); font-size: 13px; color: #56615b; }
.main-nav a { transition: color .2s; }
.main-nav a:hover { color: var(--green); }
.main-nav .nav-contact { border: 1px solid #cfd8d0; padding: 10px 15px; border-radius: 5px; color: var(--ink); }
.nav-contact span { margin-left: 13px; color: var(--green); }
.menu-toggle { display: none; }

.hero { position: relative; min-height: 585px; padding: 94px clamp(22px, 11vw, 176px) 38px; display: grid; grid-template-columns: 1.05fr .95fr; align-items: center; column-gap: 6%; border-bottom: 1px solid var(--line); }
.hero-copy { position: relative; z-index: 1; padding-bottom: 32px; }
.eyebrow { display: flex; align-items: center; gap: 10px; margin: 0 0 27px; font: 500 10px var(--mono); letter-spacing: .13em; color: #69766f; }
.status-dot { width: 6px; height: 6px; border-radius: 50%; background: #5a9477; box-shadow: 0 0 0 4px #e5eee7; }
.hero h1 { margin: 0; font-size: clamp(37px, 4.3vw, 61px); line-height: 1.32; letter-spacing: -.055em; font-weight: 700; }
.hero h1 span { color: var(--green); }
.hero-description { max-width: 440px; margin: 22px 0 29px; color: #68726d; font-size: 14px; line-height: 2; }
.hero-actions { display: flex; align-items: center; gap: 25px; }
.button { display: inline-flex; align-items: center; justify-content: center; gap: 25px; min-height: 46px; padding: 0 18px; border-radius: 4px; font-size: 13px; }
.button-primary { color: #fff; background: var(--green); transition: background .2s, transform .2s; }
.button-primary:hover { background: #1c5141; transform: translateY(-2px); }
.text-link { font-size: 12px; color: #58645d; }
.text-link span { margin-left: 8px; }
.hero-note { display: flex; align-items: center; gap: 12px; margin-top: 48px; color: #8a938e; font: 10px var(--mono); letter-spacing: .04em; }
.note-line { width: 28px; height: 1px; background: #a8b5aa; }
.hero-art { position: relative; min-height: 365px; display: grid; place-items: center; }
.art-orbit { position: absolute; border: 1px solid #dfe5dc; border-radius: 50%; transform: rotate(-26deg); }
.orbit-one { width: min(31vw, 390px); height: min(24vw, 300px); }
.orbit-two { width: min(25vw, 315px); height: min(34vw, 420px); transform: rotate(35deg); border-color: #e7eae3; }
.art-card { position: absolute; width: min(300px, 70%); height: 220px; border: 1px solid #d7dfd6; border-radius: 8px; box-shadow: 0 25px 55px #20382a12; }
.card-back { background: #e7ede5; transform: translate(26px, -22px) rotate(7deg); padding: 24px; }
.art-card-label { font: 10px var(--mono); letter-spacing: .12em; color: #77857b; }
.art-lines { display: flex; flex-direction: column; gap: 12px; margin-top: 43px; }
.art-lines i { height: 5px; border-radius: 4px; background: #d2dbd0; width: 85%; }
.art-lines i:nth-child(2) { width: 62%; }
.art-lines i:nth-child(3) { width: 74%; }
.card-front { background: #fdfefb; transform: translate(-13px, 17px) rotate(-5deg); padding: 23px; display: flex; flex-direction: column; }
.art-symbol { flex: 1; display: flex; align-items: center; justify-content: center; font-size: 98px; font-weight: 700; line-height: 1; letter-spacing: -.12em; color: #275d4b; }
.art-symbol span { color: #c0cfb6; }
.art-card-footer { display: flex; justify-content: space-between; border-top: 1px solid #e5e9e2; padding-top: 12px; color: #7a877d; font: 8px var(--mono); letter-spacing: .06em; }
.art-index { position: absolute; bottom: 5px; right: 0; font: 9px var(--mono); color: #939d95; letter-spacing: .1em; }
.art-spark { position: absolute; color: #73927a; }
.spark-one { top: 32px; right: 20px; font-size: 28px; }
.spark-two { bottom: 60px; left: 5px; font-size: 24px; font-weight: 300; }
.hero-bottom { grid-column: 1 / -1; display: flex; align-items: center; justify-content: space-between; margin-top: 34px; padding-top: 22px; border-top: 1px solid var(--line); color: #8a948d; font-size: 10px; letter-spacing: .04em; }
.hero-bottom span:last-child { font: 9px var(--mono); letter-spacing: .08em; }
.hero-bottom i { display: inline-block; width: 40px; height: 1px; margin: 0 10px 3px; background: #b7c2b8; }

.explore-section { padding: 93px clamp(22px, 11vw, 176px) 104px; }
.section-heading { display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; }
.section-heading .eyebrow { margin-bottom: 18px; }
.heading-rule { display: inline-block; width: 35px; height: 1px; background: #b3c0b4; }
.section-heading h2 { margin: 0; font-size: clamp(27px, 3vw, 39px); letter-spacing: -.04em; }
.section-intro { margin: 0 0 3px; color: #7b857e; font-size: 12px; line-height: 1.9; }
.search-box { display: flex; align-items: center; gap: 13px; height: 52px; margin: 38px 0 20px; padding: 0 16px; background: #fff; border: 1px solid #e0e5de; border-radius: 5px; }
.search-icon { font-size: 26px; line-height: 1; color: #73847a; transform: rotate(-20deg); }
.search-box input { width: 100%; min-width: 0; border: 0; outline: 0; color: var(--ink); background: transparent; font-size: 12px; }
.search-box input::placeholder { color: #a1aaa2; }
.search-box kbd { white-space: nowrap; padding: 4px 6px; border: 1px solid #e2e6df; border-radius: 3px; color: #a0a8a0; font: 9px var(--mono); }
.section-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px; }
.section-card { display: flex; flex-direction: column; min-height: 295px; padding: 22px 19px 0; border: 1px solid #e0e5dd; background: #fbfcf9; border-radius: 5px; transition: transform .2s, border-color .2s, box-shadow .2s; }
.section-card:hover { transform: translateY(-4px); border-color: #b8cabe; box-shadow: 0 14px 30px #263b2c0b; }
.card-topline { display: flex; align-items: center; justify-content: space-between; color: #9aa49b; font: 10px var(--mono); }
.card-arrow { color: #8aa092; font-size: 16px; transition: transform .2s; }
.section-card:hover .card-arrow { transform: translate(2px, -2px); }
.card-kicker { margin: 26px 0 8px; color: #688372; font: 9px var(--mono); letter-spacing: .12em; }
.section-card h3 { margin: 0; font-size: 21px; letter-spacing: -.03em; }
.card-description { min-height: 68px; margin: 12px 0 15px; color: #7a847c; font-size: 11px; line-height: 1.85; }
.tag-list { display: flex; flex-wrap: wrap; gap: 5px; margin-bottom: 22px; }
.tag-list span { padding: 5px 7px; border: 1px solid #e5e9e2; border-radius: 3px; color: #8b958c; font-size: 9px; }
.card-bottom { display: flex; justify-content: space-between; align-items: center; margin-top: auto; padding: 13px 0; border-top: 1px solid #e8ebe5; color: #6f7c72; font-size: 10px; }
.card-bottom span:last-child { color: var(--green); font-size: 14px; }
.empty-search { padding: 45px 20px; text-align: center; border: 1px dashed #d7dfd5; color: var(--muted); }
.empty-search strong { color: var(--ink); }
.empty-search p { font-size: 12px; }
.empty-search button { padding: 8px 12px; border: 1px solid #d2ddd2; border-radius: 4px; background: transparent; color: var(--green); cursor: pointer; }

.about-section { position: relative; display: grid; grid-template-columns: .35fr 1.3fr .7fr; align-items: center; gap: 40px; padding: 64px clamp(22px, 11vw, 176px); background: #eaf0e8; border-top: 1px solid #dce5db; border-bottom: 1px solid #dce5db; }
.about-mark { display: grid; place-items: center; width: 86px; height: 86px; border: 1px solid #bdcdbd; border-radius: 50%; color: #31634e; font-size: 39px; font-weight: 700; letter-spacing: -.12em; }
.about-copy .eyebrow { margin-bottom: 15px; color: #64816c; }
.about-copy h2 { margin: 0; font-size: clamp(25px, 2.8vw, 35px); line-height: 1.5; letter-spacing: -.04em; }
.about-copy > p:last-child { max-width: 500px; margin: 14px 0 0; color: #68766b; font-size: 12px; line-height: 2; }
.about-meta { align-self: end; display: flex; flex-direction: column; gap: 9px; padding-bottom: 5px; color: #788a7b; font: 9px var(--mono); letter-spacing: .08em; }
.about-meta span:last-child { color: #8c9a8d; font: 11px -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; letter-spacing: 0; }

.site-footer { min-height: 78px; display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 20px clamp(22px, 7.1vw, 112px); color: #89928b; font-size: 10px; }
.footer-brand { color: #5c6e60; font: 10px var(--mono); letter-spacing: .08em; }
.footer-brand span { margin-left: 5px; }
.site-footer > a:last-child { color: #526d5b; }

@media (min-width: 1400px) {
  .hero { min-height: 650px; }
  .section-card { min-height: 315px; padding-inline: 23px; }
}
@media (max-width: 1050px) {
  .hero { padding-inline: 7vw; column-gap: 2%; }
  .explore-section, .about-section { padding-inline: 7vw; }
  .section-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .section-card { min-height: 270px; }
  .card-description { min-height: 48px; }
  .about-section { grid-template-columns: .25fr 1.4fr; }
  .about-meta { grid-column: 2; flex-direction: row; gap: 20px; }
}
@media (max-width: 700px) {
  .site-header { height: 70px; padding-inline: 20px; }
  .brand-mark { width: 36px; height: 36px; }
  .brand-copy strong { font-size: 9px; }
  .brand-copy small { font-size: 10px; }
  .menu-toggle { display: flex; flex-direction: column; gap: 5px; padding: 10px 4px; border: 0; background: transparent; }
  .menu-toggle span { display: block; width: 23px; height: 1px; background: var(--ink); }
  .main-nav { position: absolute; z-index: 5; top: 69px; left: 0; right: 0; display: none; align-items: stretch; gap: 0; padding: 8px 20px 16px; background: var(--paper); border-bottom: 1px solid var(--line); }
  .main-nav.is-open { display: flex; flex-direction: column; }
  .main-nav a { padding: 13px 0; }
  .main-nav .nav-contact { border: 0; padding: 13px 0; }
  .hero { grid-template-columns: 1fr; padding: 67px 22px 25px; }
  .hero-copy { padding-bottom: 0; }
  .hero .eyebrow { margin-bottom: 22px; font-size: 9px; }
  .hero h1 { font-size: clamp(34px, 9vw, 49px); }
  .hero-description { margin-top: 17px; font-size: 13px; }
  .hero-note { margin-top: 30px; }
  .hero-art { min-height: 310px; margin-top: 10px; }
  .art-card { width: min(270px, 72%); height: 200px; }
  .orbit-one { width: 78vw; height: 55vw; }
  .orbit-two { width: 60vw; height: 85vw; }
  .art-index { right: 4px; bottom: 0; font-size: 8px; }
  .hero-bottom { margin-top: 26px; padding-top: 17px; font-size: 9px; }
  .hero-bottom span:last-child { font-size: 8px; }
  .hero-bottom i { width: 18px; margin-inline: 5px; }
  .explore-section { padding: 65px 20px 70px; }
  .section-heading { align-items: flex-start; flex-direction: column; gap: 15px; }
  .section-heading h2 { font-size: 29px; }
  .section-intro br { display: none; }
  .search-box { margin-top: 25px; }
  .search-box kbd { display: none; }
  .section-grid { grid-template-columns: 1fr; gap: 10px; }
  .section-card { min-height: 250px; padding-inline: 20px; }
  .card-kicker { margin-top: 20px; }
  .card-description { min-height: 0; max-width: 360px; }
  .about-section { grid-template-columns: 1fr; gap: 22px; padding: 43px 22px; }
  .about-mark { width: 60px; height: 60px; font-size: 28px; }
  .about-copy h2 { font-size: 27px; }
  .about-meta { grid-column: auto; flex-direction: column; gap: 8px; }
  .site-footer { align-items: flex-start; flex-direction: column; padding: 23px 20px; gap: 12px; }
}
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { scroll-behavior: auto !important; transition-duration: .01ms !important; animation-duration: .01ms !important; }
}

/* macOS-inspired glassmorphism layer */
:root {
  color: #17283c;
  background: #dce8f5;
  --ink: #17283c;
  --muted: #61738a;
  --green: #245ca8;
  --line: rgba(255, 255, 255, .48);
  --paper: rgba(244, 248, 255, .58);
}
body {
  background:
    radial-gradient(ellipse at 12% 8%, rgba(255, 226, 209, .95), transparent 32%),
    radial-gradient(ellipse at 83% 18%, rgba(181, 213, 247, .9), transparent 35%),
    radial-gradient(ellipse at 60% 72%, rgba(192, 222, 235, .95), transparent 38%),
    linear-gradient(135deg, #eaf0f8 0%, #cbdced 48%, #e8e0dc 100%);
  background-attachment: fixed;
}
.site-shell { background: transparent; }
.site-header {
  position: sticky;
  top: 0;
  z-index: 20;
  background: rgba(245, 249, 255, .58);
  border-bottom-color: rgba(255, 255, 255, .6);
  -webkit-backdrop-filter: blur(24px) saturate(155%);
  backdrop-filter: blur(24px) saturate(155%);
}
.brand-mark {
  background: linear-gradient(145deg, #4f91dc, #3157a4);
  border: 1px solid rgba(255, 255, 255, .5);
  box-shadow: 0 5px 18px rgba(49, 87, 164, .2);
}
.main-nav { color: #455c78; }
.main-nav .nav-contact {
  background: rgba(255, 255, 255, .36);
  border-color: rgba(255, 255, 255, .72);
  border-radius: 999px;
  box-shadow: inset 0 1px rgba(255,255,255,.55);
}
.hero {
  border-bottom-color: rgba(255, 255, 255, .45);
}
.hero h1 span { color: #326bb0; }
.status-dot { background: #4c9e8c; box-shadow: 0 0 0 4px rgba(225, 248, 243, .8); }
.button { border-radius: 999px; }
.button-primary {
  background: linear-gradient(135deg, #4c91dc, #315bb1);
  box-shadow: 0 8px 22px rgba(47, 94, 171, .2), inset 0 1px rgba(255,255,255,.3);
}
.button-primary:hover { background: linear-gradient(135deg, #3e82cc, #284d9d); }
.art-orbit { border-color: rgba(255,255,255,.58); }
.orbit-two { border-color: rgba(255,255,255,.38); }
.art-card {
  border: 1px solid rgba(255, 255, 255, .78);
  border-radius: 22px;
  box-shadow: 0 24px 65px rgba(45, 71, 107, .13), inset 0 1px rgba(255,255,255,.8);
  -webkit-backdrop-filter: blur(22px) saturate(145%);
  backdrop-filter: blur(22px) saturate(145%);
}
.card-back { background: rgba(216, 231, 246, .56); }
.art-lines i { background: rgba(110, 143, 177, .23); }
.card-front { background: linear-gradient(145deg, rgba(255,255,255,.8), rgba(231,241,252,.55)); }
.art-symbol { color: #315fa2; }
.art-symbol span { color: #90b5e1; }
.art-card-footer { border-top-color: rgba(255,255,255,.72); }
.hero-bottom { border-top-color: rgba(255,255,255,.48); }
.explore-section { position: relative; }
.search-box {
  background: rgba(255, 255, 255, .43);
  border-color: rgba(255,255,255,.8);
  border-radius: 17px;
  box-shadow: 0 8px 28px rgba(47, 73, 106, .045), inset 0 1px rgba(255,255,255,.7);
  -webkit-backdrop-filter: blur(24px) saturate(150%);
  backdrop-filter: blur(24px) saturate(150%);
}
.search-box kbd { border-color: rgba(255,255,255,.75); background: rgba(255,255,255,.36); }
.section-card {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(255,255,255,.8);
  border-radius: 22px;
  background: linear-gradient(145deg, rgba(255,255,255,.61), rgba(243,248,255,.3));
  box-shadow: 0 12px 34px rgba(49, 75, 108, .06), inset 0 1px rgba(255,255,255,.8);
  -webkit-backdrop-filter: blur(22px) saturate(145%);
  backdrop-filter: blur(22px) saturate(145%);
}
.section-card::before {
  content: '';
  position: absolute;
  width: 180px;
  height: 180px;
  top: -115px;
  right: -90px;
  border-radius: 50%;
  background: rgba(255,255,255,.24);
  filter: blur(4px);
  pointer-events: none;
}
.section-card:hover {
  transform: translateY(-5px);
  border-color: rgba(255,255,255,.98);
  box-shadow: 0 20px 42px rgba(49, 75, 108, .12), inset 0 1px rgba(255,255,255,.9);
}
.card-kicker { color: #527caf; }
.tag-list span {
  border-color: rgba(255,255,255,.8);
  background: rgba(255,255,255,.28);
  border-radius: 999px;
}
.card-bottom { border-top-color: rgba(255,255,255,.72); }
.about-section {
  background: linear-gradient(125deg, rgba(230,241,251,.68), rgba(255,246,241,.46));
  border-top-color: rgba(255,255,255,.65);
  border-bottom-color: rgba(255,255,255,.65);
  -webkit-backdrop-filter: blur(26px) saturate(145%);
  backdrop-filter: blur(26px) saturate(145%);
}
.about-mark {
  border-color: rgba(255,255,255,.9);
  background: rgba(255,255,255,.3);
  color: #315fa2;
  box-shadow: inset 0 1px rgba(255,255,255,.75), 0 8px 24px rgba(49,75,108,.06);
}
.about-copy .eyebrow { color: #527caf; }
.site-footer { color: #62768c; }
.footer-brand, .site-footer > a:last-child { color: #315f9e; }
@media (max-width: 700px) {
  .main-nav { background: rgba(240,247,255,.88); -webkit-backdrop-filter: blur(24px); backdrop-filter: blur(24px); }
  .section-card { border-radius: 19px; }
}
@media (prefers-reduced-transparency: reduce) {
  .site-header, .art-card, .search-box, .section-card, .about-section {
    -webkit-backdrop-filter: none;
    backdrop-filter: none;
  }
  .site-header { background: #eef4fb; }
  .search-box, .section-card { background: #f2f6fb; }
}

</style>
