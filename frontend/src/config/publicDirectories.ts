export type PublicDirectoryPage = {
  eyebrow: string
  title: string
  subtitle: string
  description: string
}

export type PublicDirectoryTile = {
  number: string
  title: string
  description: string
  tags: string[]
}

export const directoryPages: Record<string, PublicDirectoryPage> = {
  '/knowledge': { eyebrow: 'KNOWLEDGE & IDEAS', title: '知识广场', subtitle: '技术、经验、分析与教学', description: '汇集技术实践、经验分享、行业分析和教学文章，把值得留下的知识整理成可以持续查阅的内容。' },
  '/education-resources': { eyebrow: 'LEARNING LIBRARY', title: '教育资源', subtitle: '让学习资料更容易找到', description: '集中整理课件讲义、英语听力、网课视频链接和其他学习资料。' },
  '/homestays': { eyebrow: 'STAYS & HOMES', title: '民宿房源', subtitle: '发现值得分享的住宿空间', description: '用于展示民宿与短租房源，后续可逐步补充房源图片、位置、价格、入住说明和预订方式。' },
  '/software-tools': { eyebrow: 'SOFTWARE & TOOLS', title: '软件工具', subtitle: '把实用工具集中起来', description: '整理在线工具、桌面软件、开发运维工具、文档处理服务和自动化资源。' },
}
export const directoryTiles: Record<string, PublicDirectoryTile[]> = {
  '/knowledge': [
    { number: '01', title: '技术实践', description: '开发笔记、系统架构、部署经验与问题排查。', tags: ['编程开发', '系统运维', '实践复盘'] },
    { number: '02', title: '经验分享', description: '工具用法、工作方法与值得复用的经验。', tags: ['经验总结', '效率方法', '资源推荐'] },
    { number: '03', title: '行业分析', description: '围绕行业变化、商业逻辑和具体问题展开分析。', tags: ['行业观察', '案例分析', '商业思考'] },
    { number: '04', title: '教学文章', description: '教程、操作指南、学习路径与知识讲解。', tags: ['入门教程', '操作指南', '学习笔记'] },
  ],
  '/education-resources': [
    { number: '01', title: '课件与讲义', description: '课堂课件、教学讲义、练习题和复习材料。', tags: ['PPT', '讲义', '练习题'] },
    { number: '02', title: '听力材料', description: '英语及其他语言的听力音频、文本和训练素材。', tags: ['英语听力', '音频', '听力文本'] },
    { number: '03', title: '网课视频', description: '整理公开课程、系列教学视频和在线学习入口。', tags: ['课程链接', '系列课程', '视频学习'] },
    { number: '04', title: '学习资料', description: '电子书、参考资料、学习清单和辅助工具。', tags: ['电子资料', '学习清单', '参考链接'] },
  ],
  '/homestays': [
    { number: '01', title: '城市民宿', description: '适合城市旅行、短期出差和周末出行的住宿空间。', tags: ['城市出行', '短期住宿', '交通便利'] },
    { number: '02', title: '乡村与度假', description: '适合休闲度假、亲近自然和慢节奏旅行的房源。', tags: ['乡村体验', '度假住宿', '周边游'] },
    { number: '03', title: '短租房源', description: '按入住周期整理的短租与灵活住宿信息。', tags: ['短租', '入住周期', '配套设施'] },
    { number: '04', title: '房源信息说明', description: '后续房源详情可展示位置、价格、设施、规则和联系预订方式。', tags: ['房源图片', '价格说明', '预订方式'] },
  ],
  '/software-tools': [
    { number: '01', title: '在线效率工具', description: '无需复杂安装即可使用的在线服务与小工具。', tags: ['在线工具', '效率提升', '实用网站'] },
    { number: '02', title: '开发与运维', description: '面向开发、调试、部署和系统管理的工具。', tags: ['开发工具', '服务器', '网络诊断'] },
    { number: '03', title: '文档与格式转换', description: '处理 PDF、图片、文本、表格和常见文件格式。', tags: ['文档处理', '格式转换', '文件管理'] },
    { number: '04', title: 'AI 与自动化', description: '人工智能服务、工作流平台和重复任务自动化工具。', tags: ['AI 服务', '自动化', '工作流'] },
  ],
}
