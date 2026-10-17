# 门户开发资源合规中心管理员访问

门户开发资源合规中心包含软件许可证、资产证据与内部审核记录，因此只允许 portal.portal_users.is_admin = true 的账户访问。

## 查看账户

运行：

    sudo /opt/personal-portal/scripts/compliance/admin-access.sh list

该命令只显示脱敏邮箱、账户 ID、显示名称和管理员状态。

## 授予或撤销权限

确认账户身份后，必须使用准确的账户 UUID 和显式确认词：

    sudo /opt/personal-portal/scripts/compliance/admin-access.sh grant <账户UUID> GRANT
    sudo /opt/personal-portal/scripts/compliance/admin-access.sh revoke <账户UUID> REVOKE

当前实现不会自动把“最早注册的账户”或“内容作者账户”提升为管理员；账户归属不能仅凭显示名称推断。2026-10-17，站点所有者明确指定的账户已获授 `is_admin=true`，显示名称改为中性的“平台维护”。具体账户身份、授权前后状态及操作者依据记录在 `portal.compliance_audit_log` 中。显示名称仅用于界面展示，不隐藏数据库权限标记，也不替代审计记录。其他普通账户仍默认无版权合规管理权限。
