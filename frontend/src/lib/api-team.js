// api-team.js — 团队教练域端点封装(fork;团队模式,语义真源 muhe-fit android-app TeamApi)。
// 15 端点全部走 lib/api.js 的 api()(同源 cookie / 配对手机 Bearer 同语义)。
// 错误码契约(backend-go internal/team):404 team-none / 409 team-live-exists·owner-stale·
// team-full·team-joined / 400 team-code-invalid / 403 team-owner-only。
import { api } from './api.js'

// 身份:我的团(owner 或 member 视角);404 {code:'team-none'} = 无团。
export const getTeam = () => api('/api/team')
export const createTeam = name => api('/api/team', { method: 'POST', body: JSON.stringify({ name }) })
export const renameTeam = name => api('/api/team', { method: 'PUT', body: JSON.stringify({ name }) })

// 邀请码:一团一活码,出码旧的作废,TTL 15min(响应 expires 毫秒时间戳);DELETE 烧码。
export const mintInvite = () => api('/api/team/invite', { method: 'POST', body: '{}' })
export const revokeInvite = () => api('/api/team/invite', { method: 'DELETE' })

// 入团:码一次性兑换即烧,响应 = 确认卡 {team:{id,name}, owner:{displayName}}。
export const joinTeam = code => api('/api/team/join', { method: 'POST', body: JSON.stringify({ code }) })

// 成员面:scope/joinedAt 仅 owner 视角下发。
export const teamMembers = () => api('/api/team/members')
export const kickMember = uid => api('/api/team/members/' + encodeURIComponent(uid), { method: 'DELETE' })
export const leaveTeam = () => api('/api/team/leave', { method: 'POST' })
export const transferTeam = toUid => api('/api/team/transfer', { method: 'POST', body: JSON.stringify({ to_uid: toUid }) })

// 打卡查阅:单成员卡(60s 缓存)与全团看板(排行 weekDone 降序 streak 破平)。
export const memberSummary = uid => api('/api/team/members/' + encodeURIComponent(uid) + '/summary')
export const teamBoard = () => api('/api/team/board')

// 分析解读:双窗环比 + 规则解读 verdict + flags(hot/miss/gain/drop)。
export const teamInsights = () => api('/api/team/insights')

// 计划分发:快照中转确认制;pending = 自己的 distributed 未链数(红点)。
export const distributePlan = (uid, plan) => api('/api/team/plans', { method: 'POST', body: JSON.stringify({ uid, plan }) })
export const teamPlans = () => api('/api/team/plans')
export const ackPlan = (id, status) => api('/api/team/plans/' + encodeURIComponent(id) + '/ack', { method: 'POST', body: JSON.stringify({ status }) })
export const revokePlan = id => api('/api/team/plans/' + encodeURIComponent(id), { method: 'DELETE' })
