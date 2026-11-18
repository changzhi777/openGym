// views/admin/AdminShell.jsx — 管理后台布局壳(fork,P0)
//
// 设计依据 docs/admin-ui-design.md §3/§4:桌面侧栏(运营/系统两组导航 + 身份卡),
// 移动顶栏 + 横向页签;权限不足渲染页面内 403 面板而非重定向(§5.1)。
// 角色现实:后端(P0)只有 user.admin 布尔 —— UI 按设计的三角色建模,
// 角色来源 user.adminRoles(未来 Go 后端),缺省回落 super_admin,结构就绪。
import { useEffect, useRef, useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useStore } from '../../store/useStore.js'
import { api, authCaptcha, smsSend } from '../../lib/api.js'
import { setStepUpVerifier } from '../../lib/admin/stepUp.js'
import { adminT } from '../../lib/admin/i18n.js'
import { MOBILE } from '../../lib/mobile.js'
import Icon from '../../components/Icon.jsx'
import { Button, TextField } from '../../components/ui.jsx'
import '../../admin-console.css'

const NAV = [
  { group: '运营', items: [
    { to: '/admin', end: true, icon: 'chart', label: '概览' },
    { to: '/admin/users', icon: 'arm', label: '用户' },
    { to: '/admin/audit', icon: 'list', label: '审计' },
    { to: '/admin/roles', icon: 'key', label: '角色与权限' },
    { to: '/admin/invites', icon: 'plus', label: '邀请码' },
    { to: '/admin/revenue', icon: 'chart', label: '营收明细', supportHide: true },
  ] },
  { group: '系统', items: [
    { to: '/admin/coach', icon: 'sparkles', label: 'AI 教练' },
    { to: '/admin/coach/agent', icon: 'list', label: 'AI 治理' },
  ] },
]

const ROLE_LABEL = { super_admin: '超级管理员', ops: '运营', support: '客服(只读)' }

// StepUpModal 敏感操作二次验证弹窗(2026-11-18 审查轮:passkey 仪式已随认证重构
// 退役,改为短信码)。流程:图形验证码 → 向管理员绑定手机号发码 → 输码开 5 分钟
// 窗口。验证器契约见 lib/admin/stepUp.js(done=成功,cancel=取消)。
function StepUpModal({ done, cancel }) {
  const [cap, setCap] = useState(null) // {captchaId, svg}
  const [phone, setPhone] = useState('')
  const [capCode, setCapCode] = useState('')
  const [smsCode, setSmsCode] = useState('')
  const [sent, setSent] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const refreshCaptcha = () => {
    setSent(false)
    authCaptcha().then(setCap).catch(e => setErr(e.message))
  }
  useEffect(refreshCaptcha, [])
  const sendCode = async () => {
    setBusy(true); setErr('')
    try {
      await smsSend(phone.trim(), cap.captchaId, capCode.trim())
      setSent(true)
    } catch (e) {
      setErr(e.message)
      if (e?.data?.code === 'captcha') refreshCaptcha() // 图形码一次性:错/过期即换
    } finally { setBusy(false) }
  }
  const verify = async () => {
    setBusy(true); setErr('')
    try {
      await api('/api/admin/step-up', { method: 'POST', body: JSON.stringify({ phone: phone.trim(), code: smsCode.trim() }) })
      done()
    } catch (e) { setErr(e.message) } finally { setBusy(false) }
  }
  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,.45)', zIndex: 50, overflow: 'auto' }}
      onClick={e => { if (e.target === e.currentTarget) cancel() }}>
      <div className="adm-card" role="dialog" aria-modal="true" aria-label={adminT('二次验证')} style={{ maxWidth: 360, margin: '10vh auto' }}>
        <h3>{adminT('敏感操作二次验证')}</h3>
        <div className="muted small" style={{ marginBottom: 10 }}>{adminT('向管理员绑定的手机号发送验证码,通过后 5 分钟内无需再次验证。')}</div>
        <TextField value={phone} placeholder={adminT('管理员手机号')} onChange={e => setPhone(e.target.value)} />
        {!sent ? (
          <>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center', margin: '10px 0' }}>
              {cap && (
                <span onClick={refreshCaptcha} style={{ cursor: 'pointer', lineHeight: 0 }} title={adminT('点击刷新')}
                  aria-label={adminT('图形验证码,点击刷新')} dangerouslySetInnerHTML={{ __html: cap.svg }} />
              )}
              <TextField value={capCode} placeholder={adminT('图形验证码')} onChange={e => setCapCode(e.target.value)} />
            </div>
            <Button variant="primary" disabled={busy || !phone.trim() || capCode.trim().length < 4} onClick={sendCode}>
              {busy ? adminT('发送中…') : adminT('发送验证码')}
            </Button>
          </>
        ) : (
          <>
            <div style={{ margin: '10px 0' }}>
              <TextField value={smsCode} placeholder={adminT('6 位短信验证码')} onChange={e => setSmsCode(e.target.value)} />
            </div>
            <Button variant="primary" disabled={busy || !smsCode.trim()} onClick={verify}>
              {busy ? adminT('验证中…') : adminT('验证')}
            </Button>
          </>
        )}
        {err && <div className="adm-empty" role="alert" style={{ marginTop: 8 }}>{err}</div>}
        <div style={{ marginTop: 10 }}>
          <Button onClick={cancel}>{adminT('取消')}</Button>
        </div>
      </div>
    </div>
  )
}

export default function AdminShell() {
  const nav = useNavigate()
  const user = useStore(s => s.user)
  // 真实角色经 /api/admin/session 一次拉取(Go 栈;契约 adminSession)。
  // Node 侧 501/未就绪 → 回落旧推断(admin 布尔 → super_admin,设计 §2.6 临时映射)。
  const [sessRole, setSessRole] = useState(undefined)
  useEffect(() => {
    api('/api/admin/session').then(r => setSessRole(r.role)).catch(() => setSessRole(null))
  }, [])
  const role = sessRole ?? (user?.adminRoles?.[0] || (user?.admin ? 'super_admin' : null))
  const readonly = role === 'support'

  // step-up 弹窗注册(2026-11-18 审查轮:短信码仪式;passkey 已退役)。
  const [stepUp, setStepUp] = useState(null)
  const stepUpCtl = useRef(null)
  useEffect(() => {
    setStepUpVerifier(() => new Promise((resolve, reject) => {
      stepUpCtl.current = { resolve, reject }
      setStepUp({})
    }))
    return () => setStepUpVerifier(null)
  }, [])

  if (!role) return (
    <div className="adm-console adm-403">
      <h2>{adminT('未授权')}</h2>
      <p>{adminT('此账号没有管理权限')}</p>
    </div>
  )

  return (
    <div className="adm-console adm-shell">
      <aside className="adm-side">
        <div className="brand">沐禾 · {adminT('管理后台')}</div>
        {NAV.map(g => (
          <div key={g.group} role="navigation" aria-label={g.group}>
            <div className="nav-group">{g.group}</div>
            {g.items.filter(it => !(it.supportHide && role === 'support')).map(it => (
              <NavLink key={it.to} to={it.to} end={it.end} className="nav-item">
                <Icon name={it.icon} />{adminT(it.label)}
              </NavLink>
            ))}
          </div>
        ))}
        <div className="spacer" />
        {MOBILE && readonly && (
          <div className="adm-attention" role="status">{adminT('手机端为只读')}</div>
        )}
        <div className="adm-identity">
          <span className={'adm-badge' + (readonly ? ' readonly' : '')}>{adminT(ROLE_LABEL[role])}</span>
        </div>
        <button className="nav-item adm-back" onClick={() => nav('/home')}><Icon name="house" />{adminT('返回应用')}</button>
      </aside>
      <main className="adm-main">
        <Outlet context={{ role, readonly }} />
      </main>
      {stepUp && (
        <StepUpModal
          done={() => { setStepUp(null); stepUpCtl.current?.resolve() }}
          cancel={() => { setStepUp(null); stepUpCtl.current?.reject(new Error('step-up cancelled')) }}
        />
      )}
    </div>
  )
}
