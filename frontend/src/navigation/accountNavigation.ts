type AccountNavigationItem = {
  id: string
  label: string
  description: string
}

export type AccountNavigationGroup = {
  id: string
  label: string
  items: AccountNavigationItem[]
}

export const accountGroups: AccountNavigationGroup[] = [
  {
    id: 'personal', label: '个人中心', items: [
      { id: 'overview', label: '账户概览', description: '查看账户状态与基础信息。' },
      { id: 'profile', label: '个人资料', description: '查看当前账户的昵称和邮箱信息。' },
      { id: 'security', label: '登录与安全', description: '查看当前登录状态与账户安全信息。' },
    ],
  },
  {
    id: 'publishing', label: '发布中心', items: [
      { id: 'content-management', label: '内容管理', description: '管理发布内容、草稿与内容状态。' },
    ],
  },
  {
    id: 'housing', label: '房屋租赁中心', items: [
      { id: 'property-management', label: '房源管理', description: '集中管理房源信息与租赁资料。' },
      { id: 'room-status', label: '房态管理', description: '查看和维护房间空置、入住与出租状态。' },
    ],
  },
  {
    id: 'education', label: '教育业务中心', items: [
      { id: 'student-records', label: '学生档案', description: '管理学生基本资料与学习档案。' },
      { id: 'course-calendar', label: '课程日历', description: '查看和安排课程时间。' },
      { id: 'teaching-resources', label: '教学资源', description: '整理和管理教学资料。' },
    ],
  },
  {
    id: 'software', label: '软件工具', items: [
      { id: 'network-tools', label: '网络工具', description: '集中收纳网络检测、查询与诊断工具。' },
      { id: 'document-converters', label: '文档转换工具', description: '集中收纳常用文档格式转换工具。' },
      { id: 'add-tool', label: '添加工具', description: '添加并整理新的软件工具入口。' },
    ],
  },
  {
    id: 'workflows', label: '工作流管理', items: [
      { id: 'workflow-overview', label: '工作流总览', description: '集中查看和管理自动化工作流。' },
    ],
  },
  {
    id: 'compliance', label: '治理与合规', items: [
      { id: 'copyright-compliance', label: '门户开发资源合规中心', description: '管理门户自身的软件依赖许可证、静态资源和内部审核记录。' },
    ],
  },
  {
    id: 'system-ops', label: '系统运维', items: [
      { id: 'system-warnings', label: '系统警告', description: '查看服务器异常、文件清理失败和需要人工处理的运维警告。' },
      { id: 'system-logs', label: '系统日志', description: '实时查看服务器运行日志和 GPT 连接 SSH 的操作审计日志。' },
      { id: 'system-permissions', label: '权限管理', description: '管理门户账户角色与权限分配。' },
    ],
  },
]

export function accountGroupIcon(groupId: string): string {
  const icons: Record<string, string> = {
    personal: 'bi-person', publishing: 'bi-journal-text', housing: 'bi-house-door',
    education: 'bi-mortarboard', software: 'bi-tools', workflows: 'bi-diagram-3', compliance: 'bi-shield-check', 'system-ops': 'bi-activity',
  }
  return icons[groupId] || 'bi-grid'
}
