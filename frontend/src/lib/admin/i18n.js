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
