import type { ChildCategory, ParentCategory } from '../api/generated'

export type TaxonomyCategory = { value: ParentCategory; children: ChildCategory[] }

/** Canonical parent/child taxonomy shared by all portal forms. */
export const portalTaxonomy: TaxonomyCategory[] = [
  { value: '教育', children: ['学前教育', '小学教育', '中学教育', '高等教育', '职业教育', '家庭教育', '教学资源'] },
  { value: '年级', children: ['一年级', '二年级', '三年级', '四年级', '五年级', '六年级', '初一', '初二', '初三', '高一', '高二', '高三', '本科', '硕士'] },
  { value: '教育人力资源', children: ['事业编', '国企', '省考', '国考'] },
  { value: '旅游', children: ['住宿', '交通', '景点攻略', '旅行规划', '美食体验'] },
  { value: '网络', children: ['网络安全', '网络维护', '网络基础知识', '网络诊断', '云与服务器'] },
  { value: '系统', children: ['操作系统', '系统配置', '系统维护', '部署运维', '故障排查'] },
  { value: '编程', children: ['前端开发', '后端开发', 'Python', 'JavaScript / TypeScript', '数据库', 'API 与集成', '代码实践'] },
  { value: '软件工具', children: ['效率工具', '开发工具', '文档工具', 'AI 工具', '自动化工具'] },
]

export const taxonomyParentOptions = portalTaxonomy.map(({ value }) => ({ label: value, value }))
export const taxonomyChildren = (parent: string) => portalTaxonomy.find(category => category.value === parent)?.children ?? []
export const taxonomyChildOptions = (parent: string) => taxonomyChildren(parent).map(value => ({ label: value, value }))
export const taxonomyParentForChild = (child: ChildCategory) => portalTaxonomy.find(category => category.children.includes(child))?.value ?? ''

/** Validate the selected pair against the generated API taxonomy types. */
export function normalizeTaxonomyPair(parent: string | null, child: string | null): { parentCategory: ParentCategory | null; childCategory: ChildCategory | null } | null {
  if (!parent && !child) return { parentCategory: null, childCategory: null }
  const category = portalTaxonomy.find(item => item.value === parent)
  if (!category || !child) return null
  const matchingChild = category.children.find(item => item === child)
  if (!matchingChild) return null
  return { parentCategory: category.value, childCategory: matchingChild }
}
