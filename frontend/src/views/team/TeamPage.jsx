import { useEffect, useState } from 'react'
import { useUI } from '../../store/useUI.js'
import { t } from '../../lib/i18n.js'
import CameraScan from '../../components/CameraScan.jsx'
import { inviteCodeFromScanned, planNameOf } from '../../lib/team-load.js'
import { useTeamPlans } from './use-team-plans.js'

// 团队页(学员视角,fork;团队模式 W10 快照中转的学员侧):
// 无团 → 扫码/手输邀请码加入(码一次性,join 即入团);有团 → 我的教练 + 待接收计划
// (接收 = 快照 routines 并入自己档 + ack;婉拒 = 仅 ack)。团长视角引导去教练台
// (coach-console 的团队 tab——建团/出码/看板/分发都在那边)。
// 深链:邀请 QR 内容 {origin}/?team=code,扫开即自动入团(join 即核销,UI 明示)。
// 学员态与动作(拉取/接收/婉拒/入团/退出)单源在 use-team-plans.js——计划页的
// PlanTeamCard 共用同一套语义。
export default function TeamPage({ deepLinkCode }) {
  const { view, team, pending, busy, refresh, receive, decline, join, quit } = useTeamPlans()
  const [scanning, setScanning] = useState(false)
  const [code, setCode] = useState('')

  // 深链:扫邀请 QR 打开本页即自动入团(join 一次性核销,结果横幅明示)。
  useEffect(() => {
    const c = deepLinkCode ?? inviteCodeFromScanned(deepLinkCode)
    if (!c) return
    join(c)
  }, [deepLinkCode]) // eslint-disable-line react-hooks/exhaustive-deps

  if (view === 'loading') return <div className="team-page"><p className="muted">{t('加载中…')}</p></div>

  if (view === 'owner') {
    return (
      <div className="team-page">
        <h2>{t('我的团队')}</h2>
        <div className="sect">
          <div className="lrow">
            <div>
              <div className="lrow-t">{team.name}</div>
              <div className="muted">{t('你是团长——建团管理、邀请码、计划分发与看板在教练台')}</div>
            </div>
          </div>
        </div>
      </div>
    )
  }

  if (view === 'none') {
    return (
      <div className="team-page">
        <h2>{t('加入团队')}</h2>
        <p className="muted">{t('输入教练分享的邀请码,或扫他手机上的二维码。码 15 分钟内有效,加入后教练可以看到你的训练打卡。')}</p>
        {scanning && <CameraScan onFound={v => join(v.value)} onCancel={() => setScanning(false)} />}
        <div className="sect">
          <button className="btn" onClick={() => setScanning(s => !s)}>{scanning ? t('关闭相机') : t('扫码加入')}</button>
          <div className="lrow code-row">
            <input
              value={code}
              onChange={e => setCode(e.target.value)}
              placeholder={t('XXXX-XXXX')}
              inputMode="text"
              autoCapitalize="characters"
            />
            <button className="btn" disabled={busy || !code.trim()} onClick={() => join(code)}>{t('加入')}</button>
          </div>
        </div>
        <p className="muted">{t('想带学员?在教练台创建你的团队。')}</p>
      </div>
    )
  }

  // member
  return (
    <div className="team-page">
      <h2>{t('我的团队')}</h2>
      <div className="sect">
        <div className="lrow">
          <div>
            <div className="lrow-t">{team.name}</div>
            <div className="muted">{t('教练可以看到你的训练打卡与计划完成情况')}</div>
          </div>
        </div>
        <div className="lrow quit-row">
          <button className="btn danger" onClick={quit}>{t('退出团队')}</button>
        </div>
      </div>
      <h2>{t('待接收计划')}</h2>
      {pending.length === 0 && <p className="muted">{t('教练分发的训练计划会出现在这里。')}</p>}
      {pending.map(a => (
        <div className="sect" key={a.id}>
          <div className="lrow">
            <div>
              <div className="lrow-t">{planNameOf(a)}</div>
              <div className="muted">{(a.plan?.routines?.length || 0)} {t('个计划条目')}</div>
            </div>
            <div className="row-btns">
              <button className="btn" disabled={busy} onClick={() => receive(a)}>{t('接收')}</button>
              <button className="btn danger" disabled={busy} onClick={() => decline(a)}>{t('婉拒')}</button>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

