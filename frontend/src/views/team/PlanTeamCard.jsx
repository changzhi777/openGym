// PlanTeamCard.jsx — 计划页的团队计划同步卡(fork;团队模式 W10 学员侧的 Plan 页入口)。
// 教练分发后学员不用去「设置→我的团队」两层深处——在计划页直接看到并接收。
// 三态:pending>0 → 接收卡(接收=快照并入+ack loaded,婉拒=ack declined);
// 在团无 pending → 低调行「教练:{团名} · 我的团队→」;owner/无团/未登录 → 不渲染
// (计划页零干扰)。学员态与动作单源 use-team-plans.js(与 TeamPage 同语义)。
// nav 由宿主注入(Plan.jsx 的 useNavigate);测试注入 vi.fn()——组件零 Router 依赖。
// 样式:index.css .plan-team-card(含 .hot 接收态与 .quiet-row 低调行)。
import { useTeamPlans } from './use-team-plans.js'
import { planNameOf } from '../../lib/team-load.js'
import { t } from '../../lib/i18n.js'

export default function PlanTeamCard({ nav }) {
  const { view, team, pending, busy, receive, decline } = useTeamPlans()

  if (view === 'loading' || view === 'none' || view === 'owner') return null

  if (pending.length > 0) {
    return (
      <div className={'sect plan-team-card' + (pending.length > 0 ? ' hot' : '')}>
        <div className="lrow">
          <div>
            <div className="lrow-t plan-team-card-title">{t('教练发来了新计划')}</div>
            <div className="muted">{team.name}</div>
          </div>
        </div>
        {pending.map(a => (
          <div className="lrow" key={a.id}>
            <div>
              <div className="lrow-t">{planNameOf(a)}</div>
              <div className="muted">{(a.plan?.routines?.length || 0)} {t('个计划条目')}</div>
            </div>
            <div className="row-btns">
              <button className="btn" disabled={busy} onClick={() => receive(a)}>{t('接收')}</button>
              <button className="btn danger" disabled={busy} onClick={() => decline(a)}>{t('婉拒')}</button>
            </div>
          </div>
        ))}
      </div>
    )
  }

  // 在团、暂无待接收:低调入口(团队页有完整的教练信息/退出团/邀请深链)。
  return (
    <div className="sect plan-team-card">
      <button type="button" className="quiet-row" onClick={() => nav && nav('/team')}>
        <div>
          <div className="lrow-t">{t('教练')}：{team.name}</div>
          <div className="muted">{t('计划同步与团队详情')} →</div>
        </div>
      </button>
    </div>
  )
}
