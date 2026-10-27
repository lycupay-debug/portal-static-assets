type TableColumn = { key: string; label: string; width?: string }
export type AccountTableDefinition = { title: string; columns: TableColumn[]; note: string }

export const tableDefinitions: Record<string, AccountTableDefinition> = {
  'content-management': { title: '内容列表', columns: [{ key: 'title', label: '内容标题', width: 'minmax(220px, 2fr)' }, { key: 'type', label: '类型' }, { key: 'status', label: '状态' }, { key: 'visibility', label: '可见范围', width: 'minmax(120px, 1fr)' }, { key: 'updatedAt', label: '更新时间' }, { key: 'likeCount', label: '点赞' }, { key: 'favoriteCount', label: '收藏' }, { key: 'actions', label: '操作', width: 'minmax(330px, 2.6fr)' }], note: '内容数据源尚未接入。' },
  'property-management': { title: '房源列表', columns: [{ key: 'property', label: '房源名称', width: 'minmax(180px, 1.6fr)' }, { key: 'address', label: '地址', width: 'minmax(180px, 2fr)' }, { key: 'roomCount', label: '房间数' }, { key: 'rent', label: '租金' }, { key: 'status', label: '房源状态' }, { key: 'updatedAt', label: '更新时间' }], note: '房源数据源尚未接入。' },
  'room-status': { title: '房态列表', columns: [{ key: 'room', label: '房间', width: 'minmax(140px, 1.3fr)' }, { key: 'property', label: '所属房源', width: 'minmax(180px, 1.7fr)' }, { key: 'status', label: '当前房态' }, { key: 'tenant', label: '租客' }, { key: 'checkIn', label: '入住日期' }, { key: 'checkOut', label: '退租日期' }], note: '房态数据源尚未接入。' },
  'student-records': { title: '学生档案', columns: [{ key: 'name', label: '学生姓名' }, { key: 'studentId', label: '学员编号' }, { key: 'course', label: '课程' }, { key: 'status', label: '学习状态' }, { key: 'updatedAt', label: '更新时间' }], note: '学生档案数据源尚未接入。' },
  'course-calendar': { title: '课程安排', columns: [{ key: 'course', label: '课程名称', width: 'minmax(180px, 1.6fr)' }, { key: 'class', label: '班级' }, { key: 'teacher', label: '教师' }, { key: 'startAt', label: '开始时间' }, { key: 'endAt', label: '结束时间' }, { key: 'status', label: '状态' }], note: '课程日历数据源尚未接入。' },
  'teaching-resources': { title: '教学资源', columns: [{ key: 'title', label: '资源名称', width: 'minmax(200px, 2fr)' }, { key: 'filename', label: '文件名称', width: 'minmax(180px, 1.5fr)' }, { key: 'type', label: '类型' }, { key: 'grade', label: '年级' }, { key: 'visibility', label: '公开状态' }, { key: 'description', label: '资源说明', width: 'minmax(220px, 2fr)' }, { key: 'updatedAt', label: '更新时间' }, { key: 'actions', label: '操作' }], note: '教学资源已接入数据库。' },
  'network-tools': { title: '网络工具', columns: [{ key: 'name', label: '工具名称', width: 'minmax(180px, 1.5fr)' }, { key: 'description', label: '功能说明', width: 'minmax(220px, 2fr)' }, { key: 'url', label: '入口地址' }, { key: 'status', label: '状态' }], note: '网络工具目录尚未接入。' },
  'document-converters': { title: '文档转换工具', columns: [{ key: 'name', label: '工具名称', width: 'minmax(180px, 1.5fr)' }, { key: 'sourceFormat', label: '源格式' }, { key: 'targetFormat', label: '目标格式' }, { key: 'status', label: '状态' }, { key: 'actions', label: '操作' }], note: '文档转换工具目录尚未接入。' },
  'add-tool': { title: '工具清单', columns: [{ key: 'name', label: '工具名称', width: 'minmax(180px, 1.5fr)' }, { key: 'category', label: '分类' }, { key: 'url', label: '入口地址', width: 'minmax(220px, 2fr)' }, { key: 'status', label: '状态' }, { key: 'createdAt', label: '添加时间' }], note: '当前还没有接入工具管理数据源。' },
  'workflow-overview': { title: '工作流列表', columns: [{ key: 'name', label: '工作流名称', width: 'minmax(200px, 1.8fr)' }, { key: 'platform', label: '运行平台' }, { key: 'status', label: '运行状态' }, { key: 'lastRun', label: '最近运行' }, { key: 'result', label: '最近结果' }], note: '工作流数据源尚未接入。' },
}
export const contentTypeLabels: Record<'article' | 'tool' | 'resource' | 'project', string> = {
  article: '知识文章', tool: '在线工具', resource: '资源资料', project: '项目展示',
}
export const contentStatusLabels: Record<'draft' | 'published' | 'unpublished', string> = {
  draft: '草稿', published: '已发布', unpublished: '已下架',
}
