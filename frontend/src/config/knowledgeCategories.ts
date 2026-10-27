export type KnowledgeCategoryMenu = {
  value: string
  label: string
  children: string[]
}

export const knowledgeCategoryMenus: KnowledgeCategoryMenu[] = [
  { value: 'recent', label: '最近更新', children: [] },
  { value: 'education', label: '教育', children: ['学前教育', '小学教育', '中学教育', '高等教育', '职业教育', '家庭教育', '教学资源'] },
  { value: 'education-jobs', label: '教育人力资源', children: ['事业编', '国企', '省考', '国考'] },
  { value: 'travel', label: '旅游', children: ['住宿', '交通', '景点攻略', '旅行规划', '美食体验'] },
  { value: 'network', label: '网络', children: ['网络安全', '网络维护', '网络基础知识', '网络诊断', '云与服务器'] },
  { value: 'system', label: '系统', children: ['操作系统', '系统配置', '系统维护', '部署运维', '故障排查'] },
  { value: 'programming', label: '编程', children: ['前端开发', '后端开发', 'Python', 'JavaScript / TypeScript', '数据库', 'API 与集成', '代码实践'] },
  { value: 'software', label: '软件工具', children: ['效率工具', '开发工具', '文档工具', 'AI 工具', '自动化工具'] },
]

export const knowledgeCategorySearchTerms: Record<string, string[]> = {
      education: ['教育', '学习', '课程', '教学', '学生', '学校'], travel: ['旅游', '旅行', '住宿', '交通', '景点'],
      network: ['网络', '安全', '维护', 'DNS', 'TCP', 'HTTP'], system: ['系统', '部署', '运维', 'Linux', 'FreeBSD', 'Windows'],
      programming: ['编程', '代码', 'Python', 'JavaScript', 'TypeScript', 'API', '数据库'], software: ['软件', '工具', '效率', '自动化', 'AI'],
    }
