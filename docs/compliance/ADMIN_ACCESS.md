# 版权合规中心管理员访问

版权合规中心包含授权证据、内部审核记录及可能含有个人信息的投诉材料，因此只允许 portal.portal_users.is_admin = true 的账户访问。

## 查看账户

运行：

    sudo /opt/personal-portal/scripts/compliance/admin-access.sh list

该命令只显示脱敏邮箱、账户 ID、显示名称和管理员状态。

## 授予或撤销权限

确认账户身份后，必须使用准确的账户 UUID 和显式确认词：

    sudo /opt/personal-portal/scripts/compliance/admin-access.sh grant <账户UUID> GRANT
    sudo /opt/personal-portal/scripts/compliance/admin-access.sh revoke <账户UUID> REVOKE

当前实现不会自动把“最早注册的账户”或“内容作者账户”提升为管理员；账户归属不能仅凭显示名称推断。部署后应由站点所有者核实目标账户再授予权限。所有普通账户均默认无版权合规管理权限。
