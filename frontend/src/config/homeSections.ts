export type HomeSection = {
  id: string
  number: string
  title: string
  subtitle: string
  description: string
  tags: string[]
}

export const homeSections: HomeSection[] = [
  { id: 'knowledge', number: '01', title: '知识文章', subtitle: 'KNOWLEDGE', description: '沉淀经过整理的思考、方法与实践记录，让知识可以被持续复用。', tags: ['思考与认知', '方法论', '实践复盘'] },
  { id: 'tools', number: '02', title: '在线工具', subtitle: 'TOOLS', description: '把重复步骤变成简单工具，减少操作成本，让精力回到真正重要的问题。', tags: ['效率工具', '文本处理', '数据辅助'] },
  { id: 'resources', number: '03', title: '资源资料', subtitle: 'RESOURCES', description: '按主题组织值得留存的资料、参考链接与学习入口。', tags: ['参考资料', '学习清单', '常用链接'] },
  { id: 'projects', number: '04', title: '项目展示', subtitle: 'PROJECTS', description: '记录正在构建的系统、已完成的实践，以及过程中的关键决策。', tags: ['项目记录', '技术实践', '构建日志'] },
]
