// use-team-plans.js — 团队学员态与动作的单源 hook(fork;团队模式 W10 学员侧)。
// PlanTeamCard(计划页接收卡)与 TeamPage(团队页)共用:同一套拉取/接收/婉拒语义,
// 快照中转确认制语义真源 muhe-fit android-app TeamLoad.kt(纯函数在 lib/team-load.js)。
// 两处挂载各自实例、各自拉取(不同页面不会同时挂载,无冲突)。
import { useCallback, useEffect, useState } from 'react'
import { useStore } from '../../store/useStore.js'
import { useUI } from '../../store/useUI.js'
import { t } from '../../lib/i18n.js'
import { ackPlan, getTeam, joinTeam, leaveTeam, teamPlans } from '../../lib/api-team.js'
import { inviteCodeFromScanned, isPending, mergePlanRoutines } from '../../lib/team-load.js'

export function useTeamPlans() {
  const toast = useUI(s => s.toast)
  const update = useStore(s => s.update)
  const [view, setView] = useState('loading') // loading | none | member | owner
  const [team, setTeam] = useState(null) // {id, name, role, ownerName}
  const [pending, setPending] = useState([]) // 自己的 distributed 未链分发行
  const [busy, setBusy] = useState(false)

  const refresh = useCallback(async () => {
    try {
      const id = await getTeam()
      if (!id.team) { setView('none'); return }
      if (id.role === 'owner') { setTeam({ ...id.team, role: 'owner' }); setView('owner'); return }
      setTeam({ ...id.team, role: 'member' })
      const plans = await teamPlans()
      setPending((plans.plans || []).filter(isPending))
      setView('member')
    } catch (e) {
      if (e && e.status === 404) setView('none')
      else { toast(t('无法连接服务器')); setView('none') }
    }
  }, [toast])

  useEffect(() => { refresh() }, [refresh])

  function joinError(e) {
    if (e && e.code === 'team-code-invalid') return t('邀请码无效或已过期')
    if (e && e.code === 'team-joined') return t('你已经在这个团队里了')
    if (e && e.code === 'team-full') return t('团队人数已满')
    return t('加入失败')
  }

  // 入团:扫描文本({origin}/?team=code)或裸码;码一次性兑换即烧。
  async function join(raw) {
    const c = inviteCodeFromScanned(raw)
    if (!c) { toast(t('不是有效的团队邀请码')); return null }
    if (busy) return null
    setBusy(true)
    try {
      const r = await joinTeam(c)
      toast(t('已加入') + ' ' + (r.team?.name || '') + ' · ' + (r.owner?.displayName || ''))
      await refresh()
      return r
    } catch (e) { toast(joinError(e)); return null } finally { setBusy(false) }
  }

  // 接收:快照 routines 并入自己档(同名同练习复用)+ 盖章随同步上云 + ack;
  // 装载失败不结算(可重试);婉拒只结算。
  async function receive(a) {
    if (busy) return
    setBusy(true)
    try {
      const planRoutines = a?.plan?.routines
      if (Array.isArray(planRoutines) && planRoutines.length) {
        const { merged, added } = mergePlanRoutines(useStore.getState().S.routines || [], planRoutines)
        if (added.length) update(S => { S.routines = merged })
      }
      await ackPlan(a.id, 'loaded')
      toast(t('已接收,动作已并入你的计划'))
      await refresh()
    } catch (e) { toast(t('接收失败,请重试')) } finally { setBusy(false) }
  }

  async function decline(a) {
    if (busy) return
    setBusy(true)
    try {
      await ackPlan(a.id, 'declined')
      toast(t('已婉拒该计划'))
      await refresh()
    } catch (e) { toast(t('操作失败,请重试')) } finally { setBusy(false) }
  }

  async function quit() {
    if (!confirm(t('退出后教练将看不到你的训练数据,确定退出?'))) return
    try {
      await leaveTeam()
      toast(t('已退出团队'))
      setTeam(null); setPending([]); setView('none')
    } catch (e) { toast(t('操作失败,请重试')) }
  }

  return { view, team, pending, busy, refresh, receive, decline, join, quit }
}
