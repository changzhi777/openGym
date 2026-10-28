// views/Login.jsx — 登录/注册(认证重构 2026-10-29:手机号+短信验证码 / 微信小程序码扫码)
//
// 双 tab:手机验证码(主,注册登录一体——有号登录、无号自动建号)/ 微信扫码
// (PC 出小程序码,用户微信扫码进小程序确认,本页轮询)。邀请制实例在注册侧
// 显示邀请码输入(无号建号时随验证码提交)。访客模式与 Demo 入口保留。
// passkey/密码入口已退役(契约 deprecated;Go 生产栈无对应路由)。
import { useStore } from '../store/useStore.js'
import { useUI } from '../store/useUI.js'
import { smsSend, smsVerify, wechatQrcode, wechatPoll } from '../lib/api.js'
import { hasData } from '../store/useStore.js'
import { t } from '../lib/i18n.js'
import { DEMO, REPO } from '../lib/demo.js'
import { guestAllowed } from '../lib/guest.js'
import { useState, useRef, useEffect } from 'react'
import Icon from '../components/Icon.jsx'
import { Button } from '../components/ui.jsx'

const PHONE_RE = /^1\d{10}$/

// ── 手机验证码 tab ────────────────────────────────────────────────────────
function PhoneForm({ config, onSignedIn }) {
  const { setUser, pushState, pullState, loadConfig } = useStore()
  const inviteOnly = !!config?.invite_only
  const [phone, setPhone] = useState('')
  const [code, setCode] = useState('')
  const [invite, setInvite] = useState('')
  const [countdown, setCountdown] = useState(0)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState(null)
  const [devCode, setDevCode] = useState(null)   // dev 模式:后端把验证码回显在应答里
  const ref = useRef(null)
  useEffect(() => { setTimeout(() => ref.current?.focus(), 250) }, [])
  useEffect(() => {
    if (countdown <= 0) return
    const t = setTimeout(() => setCountdown(c => c - 1), 1000)
    return () => clearTimeout(t)
  }, [countdown])
  useEffect(() => { loadConfig() }, [loadConfig])

  const send = async () => {
    setErr(null)
    if (!PHONE_RE.test(phone)) { useUI.getState().toast(t('Enter a valid mainland mobile number')); return }
    try {
      const r = await smsSend(phone)
      setCountdown(60)
      if (r?.dev) { setDevCode(r.code || '(dev 模式,验证码见服务器日志)') }
    } catch (e) { setErr(e.message) }
  }
  const go = async () => {
    setErr(null)
    if (!PHONE_RE.test(phone)) { useUI.getState().toast(t('Enter a valid mainland mobile number')); return }
    if (!code.trim()) { useUI.getState().toast(t('Enter the SMS code')); return }
    if (inviteOnly && !invite.trim()) { useUI.getState().toast(t('An invite code is required')); return }
    setBusy(true)
    try {
      const r = await smsVerify(phone, code.trim(), invite.trim())
      setUser(r.user, { adopt: r.isNew })
      if (r.isNew && hasData(useStore.getState().S)) { await pushState(); useUI.getState().toast(t('Profile created, and this device’s data moved into it')) }
      else if (r.isNew) { await pullState(); useUI.getState().toast(t('Welcome, {0}', r.user.name)) }
      else { useUI.getState().toast(t('Welcome back, {0}', r.user.name)) }
      onSignedIn()
    } catch (e) { setErr(e.message) } finally { setBusy(false) }
  }
  return <>
    <input ref={ref} className="input" inputMode="numeric" maxLength={11}
      placeholder={t('Mobile number')} value={phone}
      onChange={e => setPhone(e.target.value.replace(/\D/g, '').slice(0, 11))} />
    <div style={{ height: 10 }} />
    <div style={{ display: 'flex', gap: 8 }}>
      <input className="input" inputMode="numeric" maxLength={6} placeholder={t('SMS code')} value={code}
        onChange={e => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
        style={{ flex: 1, letterSpacing: '.2em', textAlign: 'center' }} />
      <Button className="dim" disabled={countdown > 0 || !PHONE_RE.test(phone)} onClick={send}>
        {countdown > 0 ? `${countdown}s` : t('Send code')}
      </Button>
    </div>
    {devCode && <div className="dim small" style={{ marginTop: 6 }}>dev: {devCode}</div>}
    {inviteOnly && <>
      <div style={{ height: 10 }} />
      <input className="input" placeholder={t('Invite code')} maxLength={40} value={invite}
        onChange={e => setInvite(e.target.value.toUpperCase())}
        style={{ letterSpacing: '.14em', fontWeight: 600, textAlign: 'center' }} />
      <div className="dim small" style={{ marginTop: 6 }}>{t('This app is invite-only. Enter the code you were given.')}</div>
    </>}
    <div style={{ height: 12 }} />
    {err && <div className="dim small" style={{ color: 'var(--bad-text)', marginBottom: 8 }}>{err}</div>}
    <Button variant="primary" disabled={busy} onClick={go}>{t('Sign in / Sign up')}</Button>
  </>
}

// ── 微信扫码 tab ─────────────────────────────────────────────────────────
function WechatForm({ onSignedIn }) {
  const { setUser } = useStore()
  const [qr, setQr] = useState(null)          // { ticket, image }
  const [status, setStatus] = useState(null)  // pending / confirmed
  const [err, setErr] = useState(null)
  const load = async () => {
    setErr(null); setStatus(null); setQr(null)
    try {
      const r = await wechatQrcode()
      setQr(r)
    } catch (e) { setErr(e.message) }
  }
  useEffect(() => { load() }, [])
  useEffect(() => {
    if (!qr?.ticket) return
    let stop = false
    const iv = setInterval(async () => {
      try {
        const r = await wechatPoll(qr.ticket)
        if (stop) return
        if (r.status === 'confirmed') {
          clearInterval(iv); setStatus('confirmed')
          if (r.user) { setUser(r.user); useUI.getState().toast(t('Welcome back, {0}', r.user.name)); onSignedIn() }
        }
      } catch { /* 单次轮询失败静默,下一轮再试 */ }
    }, 2000)
    return () => { stop = true; clearInterval(iv) }
  }, [qr?.ticket])

  return <>
    <div className="muted small" style={{ marginBottom: 12 }}>
      {t('Scan with WeChat, confirm in the 沐禾健身 mini program, and this page signs in.')}
    </div>
    {err && <div className="card small muted" style={{ textAlign: 'start' }}>{err}</div>}
    {qr?.image && <img alt="WeChat QR" src={'data:image/png;base64,' + qr.image}
      style={{ width: 220, height: 220, borderRadius: 12, background: '#fff', padding: 8 }} />}
    {status === 'confirmed'
      ? <div className="ok" style={{ marginTop: 10 }}>{t('Confirmed on WeChat')}</div>
      : qr && <div className="dim small" style={{ marginTop: 10 }}>{t('Waiting for scan…')}</div>}
    <div style={{ height: 10 }} />
    <Button className="dim" onClick={load}>{t('Refresh')}</Button>
  </>
}

export default function Login() {
  const { setUser, setGuest } = useStore()
  const config = useStore(s => s.config)
  const canGuest = guestAllowed(config)
  const [tab, setTab] = useState('phone')
  const head = <>
    <div style={{ fontSize: 54, display: 'flex', justifyContent: 'center', color: 'var(--acc)' }}><Icon name="dumbbell" /></div>
    <h1 style={{ fontSize: 34, fontWeight: 700, letterSpacing: '-.028em', margin: '10px 0 4px' }}>{t('openGym')}</h1>
  </>
  const wrap = { display: 'flex', flexDirection: 'column', justifyContent: 'center', minHeight: '78vh', textAlign: 'center' }

  // Demo build: no backend to sign in against — the only way in is the local guest profile.
  if (DEMO) return (
    <div className="narrow" style={wrap}>
      {head}
      <div className="muted" style={{ marginBottom: 30 }}>{t('Live demo. Everything stays in this browser.')}</div>
      <Button variant="primary" icon="play" onClick={() => setGuest(true)}>{t('Start the demo')}</Button>
      <div className="card small muted" style={{ textAlign: 'start', marginTop: 16 }}>
        {t('This demo runs entirely in your browser on example data. Nothing is sent anywhere. Passkey sign-in and sync across your devices come with the openGym server, which you get by self-hosting it.')}
      </div>
      <div className="dim small" style={{ marginTop: 22, lineHeight: 1.6 }}>
        <a href={REPO} target="_blank" rel="noopener">{t('Self-host it in a minute →')}</a>
      </div>
    </div>
  )

  const signedIn = () => {}   // 各表单自行 toast;onSignedIn 预留路由外动作

  return (
    <div className="narrow" style={wrap}>
      {head}
      <div className="muted" style={{ marginBottom: 24 }}>{t('Your workouts. Your weights. Your profile.')}</div>
      <div style={{ display: 'flex', gap: 8, justifyContent: 'center', marginBottom: 16 }}>
        {[['phone', t('Mobile sign-in'), 'phone'], ['wechat', t('WeChat scan'), 'qr']].map(([k, label]) => (
          <button key={k} onClick={() => setTab(k)}
            style={{
              padding: '8px 18px', borderRadius: 20, fontSize: 14, cursor: 'pointer',
              border: `1px solid ${tab === k ? 'var(--acc)' : 'var(--hair)'}`,
              color: tab === k ? 'var(--acc)' : 'var(--label-2)', background: 'none', fontWeight: 600,
            }}>{label}</button>
        ))}
      </div>
      <div style={{ textAlign: 'start', maxWidth: 420, margin: '0 auto', width: '100%' }}>
        {tab === 'phone'
          ? <PhoneForm config={config} onSignedIn={signedIn} />
          : <WechatForm onSignedIn={signedIn} />}
      </div>
      {canGuest && <>
        <div style={{ height: 14 }} />
        <Button variant="ghost" className="dim" onClick={() => setGuest(true)}>{t('Continue without account')}</Button>
      </>}
      <div className="dim small" style={{ marginTop: 26, lineHeight: 1.5 }}>
        {t('Each profile keeps its own plan, workouts & body weight.')}
      </div>
    </div>
  )
}
