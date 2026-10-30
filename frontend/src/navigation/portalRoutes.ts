export const sectionRoutes: Record<string, string> = {
  overview: '/account', profile: '/profile', security: '/security',
  'content-management': '/content-management', 'property-management': '/housing/property-management',
  'room-status': '/housing/room-status', 'student-records': '/education/student-records',
  'course-calendar': '/education/course-calendar', 'teaching-resources': '/education/teaching-resources',
  'network-tools': '/tools/network', 'document-converters': '/tools/document-converters',
  'add-tool': '/tools/add', 'workflow-overview': '/workflows', 'copyright-compliance': '/account/compliance',
  'system-warnings': '/account/system-ops/warnings', 'system-logs': '/account/system-ops/logs',
  'system-permissions': '/account/system-ops/permissions',
}
export const routeSections = Object.fromEntries(Object.entries(sectionRoutes).map(([id, path]) => [path, id])) as Record<string, string>
export const routeAliases: Record<string, string> = {
  '/首页': '/', '/个人中心': '/account', '/个人资料': '/profile', '/登录与安全': '/security',
  '/登录': '/login', '/注册': '/register', '/内容管理': '/content-management',
  '/房源管理': '/housing/property-management', '/房态管理': '/housing/room-status',
  '/学生档案': '/education/student-records', '/课程日历': '/education/course-calendar',
  '/教学资源': '/education/teaching-resources', '/网络工具': '/tools/network',
  // Legacy bookmark compatibility only; the retired policy page now resolves to RBAC management.
  '/account/system-ops/policy': '/account/system-ops/permissions',
  '/文档转换': '/tools/document-converters', '/添加工具': '/tools/add', '/工作流管理': '/workflows',
}
