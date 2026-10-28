// lib/admin/i18n.js — 管理后台独立文案包(fork)
//
// 设计依据 docs/admin-ui-design.md §7:后台文案独立成包,不进 frontend/src/locales/
// 的 20 个用户端语言包 —— 减少上游合并冲突,且后台操作术语(审计/配额/step-up)
// 与用户端语境不同。zh 默认,en 保留;后续语言按需加。
const packs = {
  zh: {
    '管理后台': '管理后台', '概览': '概览', '用户': '用户', '审计': '审计',
    '角色与权限': '角色与权限', '邀请码': '邀请码', 'AI 教练': 'AI 教练', '返回应用': '返回应用',
    '未授权': '未授权', '此账号没有管理权限': '此账号没有管理权限',
    '需要更高权限': '需要更高权限', '此操作需要 {0} 或更高角色': '此操作需要 {0} 或更高角色',
    '用户总数': '用户总数', '今日活跃': '今日活跃', '累计训练': '累计训练',
    'AI 调用(本月)': 'AI 调用(本月)', '营收(本月)': '营收(本月)',
    '实时训练中': '实时训练中', '最近管理操作': '最近管理操作', '需要关注': '需要关注',
    '暂无数据': '暂无数据', '加载中…': '加载中…', '加载失败': '加载失败', '重试': '重试',
    '刷新': '刷新', '刚刚': '刚刚', '{0} 分钟前': '{0} 分钟前', '{0} 小时前': '{0} 小时前', '{0} 天前': '{0} 天前',
    '待后端支持': '待后端支持', '此功能等待后端(Go 重构)就绪后开放': '此功能等待后端(Go 重构)就绪后开放',
    '超级管理员': '超级管理员', '运营': '运营', '客服(只读)': '客服(只读)',
    '验证有效期': '验证有效期', '{0} 分钟': '{0} 分钟',
    '手机端为只读': '手机端为只读', '敏感操作请在浏览器中打开后台完成': '敏感操作请在浏览器中打开后台完成',
    '复制后台链接': '复制后台链接', '已复制': '已复制',
    '账户操作': '账户操作', '全部': '全部', '搜索': '搜索', '停用': '停用', '启用': '启用', '重置码': '重置码',
    '删除用户': '删除用户', '删除后不可恢复': '删除后不可恢复',
    '输入账户名以确认': '输入账户名以确认', '创建重置码': '创建重置码',
    '训练记录': '训练记录', '体重记录': '体重记录', '最近同步': '最近同步',
    '停用后该用户无法登录': '停用后该用户无法登录', '启用后该用户可重新登录': '启用后该用户可重新登录',
    '授予管理员': '授予管理员', '选择用户': '选择用户', '搜索姓名或邮箱': '搜索姓名或邮箱', '选择角色': '选择角色',
    '授予后立即生效，并记录到审计日志': '授予后立即生效，并记录到审计日志', '授予角色': '授予角色',
    '变更角色': '变更角色', '撤销管理员': '撤销管理员', '仅服务器配置可调整': '仅服务器配置可调整',
    '超级管理员只能通过服务器配置调整': '超级管理员只能通过服务器配置调整',
    '全部权限': '全部权限', '删号 · 角色 · 清日志': '删号 · 角色 · 清日志',
    '用户 / 配额 / 停用 / 邀请': '用户 / 配额 / 停用 / 邀请', '敏感操作需二次验证': '敏感操作需二次验证',
    '不能删号 / 管角色': '不能删号 / 管角色', '只读查看': '只读查看', '不能修改任何数据': '不能修改任何数据',
    '不可见营收与 IP': '不可见营收与 IP', '成员': '成员', '授予人': '授予人', '授予时间': '授予时间', '操作': '操作',
    '此页面仅超级管理员可见': '此页面仅超级管理员可见', '返回概览': '返回概览', '你的角色是': '你的角色是',
    '变更角色为': '变更角色为', '确认撤销': '确认撤销', '撤销后该用户失去后台访问': '撤销后该用户失去后台访问',
    '今日新增': '今日新增', '已停用': '已停用', '正常': '正常', '普通用户': '普通用户',
    '邮箱': '邮箱', '加入时间': '加入时间', '排序': '排序', '状态': '状态', '显示完整': '显示完整',
    '账户概览': '账户概览', '配额': '配额', '训练明细': '训练明细', '账户操作': '账户操作',
    'AI 教练(本月)': 'AI 教练(本月)', '本月已用': '本月已用', '留空 = 实例默认': '留空 = 实例默认',
    '有未保存的更改': '有未保存的更改', '放弃': '放弃', '保存': '保存', '下载数据': '下载数据',
    '训练明细包含个人健康数据。验证后可查看。': '训练明细包含个人健康数据。验证后可查看。',
    '使用通行密钥查看': '使用通行键查看', '已验证': '已验证', '重新锁定': '重新锁定',
    '清空审计日志': '清空审计日志', '输入"清空"以确认': '输入"清空"以确认', '保留': '保留', '天': '天',
    '搜索操作者、目标或事件': '搜索操作者、目标或事件', '展开详情': '展开详情', '收起': '收起',
    '来源 IP': '来源 IP', '事件': '事件', '目标': '目标', '结果': '结果', '成功': '成功', '失败': '失败',
  },
  en: {
    '管理后台': 'Admin console', '概览': 'Overview', '用户': 'Users', '审计': 'Audit',
    '角色与权限': 'Roles & access', '邀请码': 'Invites', 'AI 教练': 'AI coach', '返回应用': 'Back to app',
    '未授权': 'Not authorized', '此账号没有管理权限': 'This account has no admin access',
    '需要更高权限': 'Higher role required', '此操作需要 {0} 或更高角色': 'This action needs {0} or above',
    '用户总数': 'Total users', '今日活跃': 'Active today', '累计训练': 'Workouts all-time',
    'AI 调用(本月)': 'AI calls (month)', '营收(本月)': 'Revenue (month)',
    '实时训练中': 'Training now', '最近管理操作': 'Recent admin actions', '需要关注': 'Needs attention',
    '暂无数据': 'No data yet', '加载中…': 'Loading…', '加载失败': 'Failed to load', '重试': 'Retry',
    '刷新': 'Refresh', '刚刚': 'just now', '{0} 分钟前': '{0} min ago', '{0} 小时前': '{0} h ago', '{0} 天前': '{0} d ago',
    '待后端支持': 'Awaiting backend', '此功能等待后端(Go 重构)就绪后开放': 'This opens when the Go backend lands',
    '超级管理员': 'super admin', '运营': 'ops', '客服(只读)': 'support (read-only)',
    '验证有效期': 'Step-up valid for', '{0} 分钟': '{0} min',
    '手机端为只读': 'Read-only in the app', '敏感操作请在浏览器中打开后台完成': 'Open the console in a browser for sensitive actions',
    '复制后台链接': 'Copy console link', '已复制': 'Copied',
    '账户操作': 'Account actions', '全部': 'All', '搜索': 'Search', '停用': 'Disable', '启用': 'Enable', '重置码': 'Reset code',
    '删除用户': 'Delete user', '删除后不可恢复': 'This cannot be undone',
    '输入账户名以确认': 'Type the account name to confirm', '创建重置码': 'Create reset code',
    '训练记录': 'Workouts', '体重记录': 'Weigh-ins', '最近同步': 'Last sync',
    '停用后该用户无法登录': 'They cannot sign in while disabled', '启用后该用户可重新登录': 'They can sign in again',
    '授予管理员': 'Grant admin', '选择用户': 'Pick a user', '搜索姓名或邮箱': 'Search name or email', '选择角色': 'Pick a role',
    '授予后立即生效，并记录到审计日志': 'Takes effect immediately; written to the audit trail', '授予角色': 'Grant role',
    '变更角色': 'Change role', '撤销管理员': 'Revoke admin', '仅服务器配置可调整': 'Server-config only',
    '超级管理员只能通过服务器配置调整': 'super_admin can only be changed via server config',
    '全部权限': 'Full access', '删号 · 角色 · 清日志': 'Delete · roles · log purge',
    '用户 / 配额 / 停用 / 邀请': 'Users / quota / disable / invites', '敏感操作需二次验证': 'Sensitive actions need step-up',
    '不能删号 / 管角色': 'No deletion / role admin', '只读查看': 'Read-only', '不能修改任何数据': 'Cannot modify data',
    '不可见营收与 IP': 'No revenue or IP', '成员': 'Members', '授予人': 'Granted by', '授予时间': 'Granted at', '操作': 'Actions',
    '此页面仅超级管理员可见': 'super_admin only', '返回概览': 'Back to overview', '你的角色是': 'Your role is',
    '变更角色为': 'Change role to', '确认撤销': 'Confirm revoke', '撤销后该用户失去后台访问': 'They lose console access',
    '今日新增': 'New today', '已停用': 'Disabled', '正常': 'Active', '普通用户': 'Member',
    '邮箱': 'Email', '加入时间': 'Joined', '排序': 'Sort', '状态': 'Status', '显示完整': 'Show',
    '账户概览': 'Account', '配额': 'Quota', '训练明细': 'Workout detail', '账户操作': 'Account actions',
    'AI 教练(本月)': 'AI coach (month)', '本月已用': 'Used this month', '留空 = 实例默认': 'Empty = instance default',
    '有未保存的更改': 'Unsaved changes', '放弃': 'Discard', '保存': 'Save', '下载数据': 'Download data',
    '训练明细包含个人健康数据。验证后可查看。': 'Workout detail is health data. Verify to view.',
    '使用通行密钥查看': 'Verify with passkey', '已验证': 'Verified', '重新锁定': 'Lock again',
    '清空审计日志': 'Clear audit log', '输入"清空"以确认': 'Type "clear" to confirm', '保留': 'Retention', '天': 'd',
    '搜索操作者、目标或事件': 'Search actor, target or event', '展开详情': 'Details', '收起': 'Collapse',
    '来源 IP': 'Source IP', '事件': 'Event', '目标': 'Target', '结果': 'Result', '成功': 'OK', '失败': 'Fail',
  },
}
const lang = () => {
  const s = (() => { try { return JSON.parse(localStorage.getItem('gym_state_v1') || '{}') } catch (e) { return {} } })()
  return s?.state?.settings?.lang || 'zh'
}
export const adminT = (key, ...args) => {
  const p = packs[lang()] || packs.zh
  let v = p[key] ?? packs.en[key] ?? key
  args.forEach((a, i) => { v = v.replaceAll('{' + i + '}', a) })
  return v
}
export const adminWhen = ts => {
  if (!ts) return ''
  const s = Math.max(0, (Date.now() - ts) / 1000)
  if (s < 60) return adminT('刚刚')
  if (s < 3600) return adminT('{0} 分钟前', Math.floor(s / 60))
  if (s < 86400) return adminT('{0} 小时前', Math.floor(s / 3600))
  return adminT('{0} 天前', Math.floor(s / 86400))
}
