// views/Login.jsx — 登录/注册(认证重构:手机号+短信验证码 / 微信小程序码扫码)
//
// 设计规格(2026-10-29 十项决议):全屏沉浸式浅色(雾白→淡绿灰渐变,无卡片)、
// LOGO 卡片居中、手机 tab:内嵌 +86 前缀 → 图形验证码(SVG 数字码,发码前置闸) →
// 全宽绿钮『获取短信验证码』→ 6 格分段验证码(自动跳格/回退/粘贴) → 品牌绿 CTA；
// 微信 tab:小程序码+轮询；双通道按钮平级互切；错误=字段级红字。全中文文案。
import { useStore } from '../store/useStore.js'
import { useUI } from '../store/useUI.js'
import { smsSend, smsVerify, wechatQrcode, wechatPoll, authCaptcha, bindPhone } from '../lib/api.js'
import { hasData } from '../store/useStore.js'
import { DEMO, REPO } from '../lib/demo.js'
import { guestAllowed } from '../lib/guest.js'
import { useState, useRef, useEffect } from 'react'
import Icon from '../components/Icon.jsx'
import { Button } from '../components/ui.jsx'

const PHONE_RE = /^1\d{10}$/

// 浅色系设计 token(Q8 色板)。
const C = {
  bgTop: '#f2f2f7', bgBottom: '#e9efe9', ink: '#1c1c1e', sub: '#6e6e73',
  green: '#248a3d', greenDark: '#1e7534', hair: '#c7c7cc', white: '#ffffff',
  red: '#c62828',
}
const loginInput = {
  width: '100%', background: C.white, color: C.ink,
  border: `1.5px solid ${C.hair}`, borderRadius: 12,
  padding: '13px 14px', fontSize: 16, outline: 'none',
}
const focusInput = e => { e.currentTarget.style.borderColor = C.green }
const blurInput = e => { e.currentTarget.style.borderColor = C.hair }
const fieldErr = msg => <div style={{ color: C.red, fontSize: 12.5, marginTop: 5, textAlign: 'left' }}>{msg}</div>
const label = txt => <div style={{ fontSize: 13.5, fontWeight: 600, color: C.ink, margin: '14px 0 6px', textAlign: 'left' }}>{txt}</div>

// ── 6 格分段验证码(自动跳格/回退/粘贴分发;获焦绿边+微放大) ─────────────────
function SixDigits({ value, onChange }) {
  const refs = useRef([])                    // 稳定容器:重渲染/卸载时 ref 回调不错位
  const [focusIdx, setFocusIdx] = useState(-1)
  const digits = Array.from({ length: 6 }, (_, i) => value[i] || '')
  const setDigit = (i, ch) => {
    const next = (value.slice(0, i) + ch + value.slice(i + 1)).replace(/\D/g, '').slice(0, 6)
    onChange(next)
  }
  const onKey = (i, e) => {
    if (e.key === 'Backspace' && !digits[i] && i > 0) { refs.current[i - 1]?.focus(); e.preventDefault() }
    if (e.key === 'ArrowLeft' && i > 0) refs.current[i - 1]?.focus()
    if (e.key === 'ArrowRight' && i < 5) refs.current[i + 1]?.focus()
  }
  const onInput = (i, e) => {
    const ch = e.target.value.replace(/\D/g, '').slice(-1)
    if (ch) { setDigit(i, ch); if (i < 5) refs.current[i + 1]?.focus() }
    else setDigit(i, '')
  }
  const onPaste = e => {
    e.preventDefault()
    const pasted = (e.clipboardData.getData('text') || '').replace(/\D/g, '').slice(0, 6)
    if (pasted) { onChange(pasted); refs.current[Math.min(pasted.length, 5)]?.focus() }
  }
  return (
    <div style={{ display: 'flex', gap: 8, justifyContent: 'space-between' }} onPaste={onPaste}>
      {digits.map((d, i) => (
        <input key={i} ref={el => (refs.current[i] = el)} value={d} inputMode="numeric" maxLength={1}
          onFocus={() => setFocusIdx(i)} onBlur={() => setFocusIdx(-1)}
          onChange={e => onInput(i, e)} onKeyDown={e => onKey(i, e)}
          style={{
            width: '100%', textAlign: 'center', fontSize: 22, fontWeight: 700, color: C.ink,
            background: C.white, border: `1.5px solid ${focusIdx === i ? C.green : C.hair}`,
            borderRadius: 12, padding: '10px 0', outline: 'none', flex: 1,
            transform: focusIdx === i ? 'scale(1.04)' : 'none', transition: 'border-color .12s, transform .12s',
          }} />
      ))}
    </div>
  )
}

// ── 手机号登录(主通道:图形码 → 获取短信 → 6 格 → 登录) ────────────────────
function PhoneForm({ config, onSignedIn }) {
  const { setUser, pushState, pullState, loadConfig } = useStore()
  const inviteOnly = !!config?.invite_only
  const [phone, setPhone] = useState('')
  const [cap, setCap] = useState(null)      // {captchaId, svg}
  const [capCode, setCapCode] = useState('')
  const [sent, setSent] = useState(false)   // 短信已发 → 显示 6 格 + 登录钮
  const [countdown, setCountdown] = useState(0)
  const [digits, setDigits] = useState('')
  const [invite, setInvite] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState(null)      // {msg, field}
  const phoneRef = useRef(null)
  useEffect(() => { setTimeout(() => phoneRef.current?.focus(), 250) }, [])
  useEffect(() => { loadConfig() }, [loadConfig])
  // 图形验证码进页即拉(发短信的前置人机闸,决议 Q3)。
  useEffect(() => { refreshCaptcha() }, [])
  useEffect(() => {
    if (countdown <= 0) return
    const t = setTimeout(() => setCountdown(c => c - 1), 1000)
    return () => clearTimeout(t)
  }, [countdown])

  const refreshCaptcha = async () => {
    setErr(null); setCapCode('')
    try { setCap(await authCaptcha()) } catch (e) { setErr({ msg: e.message, field: 'captcha' }) }
  }
  const getCode = async () => {
    setErr(null)
    if (!PHONE_RE.test(phone)) { setErr({ msg: '请输入有效的中国大陆手机号', field: 'phone' }); return }
    if (capCode.length !== 4) { setErr({ msg: '请输入图形验证码', field: 'captchaCode' }); return }
    try {
      await smsSend(phone, cap.captchaId, capCode)
      setSent(true); setDigits(''); setCountdown(60)
    } catch (e) {
      const field = e?.data?.code === 'captcha' ? 'captcha' : 'captchaCode'
      setErr({ msg: e.message, field })
      if (e?.data?.code === 'captcha') refreshCaptcha()
    }
  }
  const signIn = async () => {
    setErr(null)
    if (digits.length < 6) { setErr({ msg: '请输入 6 位短信验证码', field: 'digits' }); return }
    if (inviteOnly && !invite.trim()) { setErr({ msg: '需要邀请码', field: 'invite' }); return }
    setBusy(true)
    try {
      const r = await smsVerify(phone, digits, invite.trim())
      setUser(r.user, { adopt: r.isNew })
      if (r.isNew && hasData(useStore.getState().S)) { await pushState(); useUI.getState().toast('账号已创建，本机数据已并入') }
      else if (r.isNew) { await pullState(); useUI.getState().toast('欢迎，' + r.user.name) }
      else { useUI.getState().toast('欢迎回来，' + r.user.name) }
      onSignedIn()
    } catch (e) { setErr({ msg: e.message, field: 'digits' }) } finally { setBusy(false) }
  }

  return <>
    {label('手机号')}
    <div style={{ display: 'flex', alignItems: 'stretch', background: C.white,
      border: `1.5px solid ${err?.field === 'phone' ? C.red : C.hair}`, borderRadius: 12, overflow: 'hidden' }}>
      <span style={{ padding: '13px 12px', background: '#eef1ee', color: C.sub, fontSize: 15,
        borderRight: `1.5px solid ${C.hair}`, userSelect: 'none' }}>+86</span>
      <input ref={phoneRef} inputMode="numeric" maxLength={11} placeholder="请输入手机号" value={phone}
        onChange={e => setPhone(e.target.value.replace(/\D/g, '').slice(0, 11))}
        onFocus={focusInput} onBlur={blurInput}
        style={{ flex: 1, border: 'none', outline: 'none', fontSize: 16, padding: '13px 14px', background: 'transparent', color: C.ink }} />
    </div>
    {err?.field === 'phone' && fieldErr(err.msg)}

    {label('图形验证码')}
    <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
      {cap && <div dangerouslySetInnerHTML={{ __html: cap.svg }}
        style={{ width: 160, height: 48, borderRadius: 8, overflow: 'hidden', cursor: 'pointer', flexShrink: 0,
          border: `1.5px solid ${err?.field === 'captcha' ? C.red : C.hair}` }}
        onClick={refreshCaptcha} title="点击刷新" />}
      <input inputMode="numeric" maxLength={4} placeholder="输入 4 位数字" value={capCode}
        onChange={e => setCapCode(e.target.value.replace(/\D/g, '').slice(0, 4))}
        onFocus={focusInput} onBlur={blurInput}
        style={{ ...loginInput, flex: 1, borderColor: err?.field === 'captcha' ? C.red : C.hair }} />
    </div>
    {err?.field === 'captcha' && fieldErr(err.msg)}

    {!sent ? <>
      <div style={{ height: 18 }} />
      <button onClick={getCode} style={{ width: '100%', padding: '14px 0', border: 'none', borderRadius: 14,
        background: C.green, color: '#fff', fontSize: 16, fontWeight: 700, cursor: 'pointer' }}
        onMouseEnter={e => (e.currentTarget.style.background = C.greenDark)}
        onMouseLeave={e => (e.currentTarget.style.background = C.green)}>
        获取短信验证码
      </button>
    </> : <>
      {label('短信验证码')}
      <SixDigits value={digits} onChange={setDigits} />
      <div style={{ display: 'flex', justifyContent: 'space-between', margin: '8px 0 14px' }}>
        <span style={{ fontSize: 12.5, color: C.sub }}>
          {countdown > 0 ? `已发送（${countdown}s 后可重发）` : '没收到？'}
        </span>
        {countdown <= 0 && <button style={{ background: 'none', border: 'none', color: C.green,
          fontWeight: 600, cursor: 'pointer', fontSize: 13 }} onClick={getCode}>重新获取</button>}
      </div>
    </>}

    {sent && <>
      {err?.field === 'digits' && fieldErr(err.msg)}
      {inviteOnly && <>
        <div style={{ height: 4 }} />
        <input className="input" placeholder="邀请码" maxLength={40} value={invite}
          onChange={e => setInvite(e.target.value.toUpperCase())}
          onFocus={focusInput} onBlur={blurInput}
          style={{ ...loginInput, letterSpacing: '.14em', fontWeight: 600, textAlign: 'center' }} />
        <div className="dim small" style={{ marginTop: 6 }}>本应用为邀请制，请输入你收到的邀请码。</div>
      </>}
      <div style={{ height: 16 }} />
      <button onClick={signIn} disabled={busy || digits.length < 6}
        style={{ width: '100%', padding: '15px 0', border: 'none', borderRadius: 14,
          background: busy || digits.length < 6 ? '#a8d4b0' : C.green, color: '#fff',
          fontSize: 16.5, fontWeight: 700, cursor: busy || digits.length < 6 ? 'default' : 'pointer' }}>
        登录 / 注册
      </button>
    </>}

    {err && !err.field && fieldErr(err.msg)}
  </>
}

// ── 微信扫码 tab ─────────────────────────────────────────────────────────
function WechatForm({ onSwitchBack }) {
  const { setUser } = useStore()
  const [qr, setQr] = useState(null)
  const [status, setStatus] = useState(null)
  const [err, setErr] = useState(null)
  const load = async () => {
    setErr(null); setStatus(null); setQr(null)
    try { setQr(await wechatQrcode()) } catch (e) { setErr(e.message) }
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
          if (r.user) { setUser(r.user); useUI.getState().toast('欢迎回来，' + r.user.name); onSwitchBack() }
        }
      } catch { /* 单次轮询失败静默 */ }
    }, 2000)
    return () => { stop = true; clearInterval(iv) }
  }, [qr?.ticket])

  return <>
    <div style={{ fontSize: 14, color: C.sub, marginBottom: 12 }}>
      微信扫描二维码，在「沐禾健身」小程序中确认，网页将自动登录
    </div>
    {err && <div className="dim small" style={{ color: '#c62828' }}>{err}</div>}
    {qr?.image && <img alt="微信扫码" src={'data:image/png;base64,' + qr.image}
      style={{ width: 220, height: 220, borderRadius: 12, background: '#fff', padding: 8,
        border: `1px solid ${C.hair}`, margin: '0 auto', display: 'block' }} />}
    {status === 'confirmed'
      ? <div style={{ color: C.green, fontWeight: 700, marginTop: 10 }}>已在微信中确认 ✓</div>
      : qr && <div style={{ color: C.sub, fontSize: 13, marginTop: 10 }}>等待扫码中…</div>}
    <div style={{ height: 10 }} />
    <button onClick={load} style={{ background: 'none', border: 'none', color: C.green,
      fontWeight: 600, cursor: 'pointer', fontSize: 13 }}>刷新二维码</button>
  </>
}

export default function Login() {
  const { setUser, setGuest } = useStore()
  const config = useStore(s => s.config)
  const canGuest = guestAllowed(config)
  const [channel, setChannel] = useState('phone')   // phone | wechat 双按钮平级(Q7)
  const head = <>
    <img src="/icon-192.png" alt="沐禾健身" width={88} height={88}
      style={{ borderRadius: 22, margin: '0 auto', display: 'block', boxShadow: '0 6px 24px rgba(36,138,61,.25)' }} />
    <h1 style={{ fontSize: 32, fontWeight: 800, letterSpacing: '-.02em', margin: '14px 0 4px', color: '#1c1c1e' }}>沐禾健身</h1>
    <div style={{ fontSize: 13, fontWeight: 600, letterSpacing: '.22em', color: '#248a3d', textTransform: 'uppercase' }}>MUHE FIT</div>
  </>
  const wrap = {
    display: 'flex', flexDirection: 'column', justifyContent: 'center', textAlign: 'center',
    minHeight: '100dvh', background: 'linear-gradient(180deg, #f2f2f7 0%, #e9efe9 100%)', color: '#1c1c1e',
    ['data-theme']: 'light',
  }

  // Demo build: no backend to sign in against — the only way in is the local guest profile.
  if (DEMO) return (
    <div className="narrow" style={wrap}>
      {head}
      <div className="muted" style={{ marginBottom: 30 }}>Live demo. Everything stays in this browser.</div>
      <Button variant="primary" icon="play" onClick={() => setGuest(true)}>Start the demo</Button>
      <div className="card small muted" style={{ textAlign: 'start', marginTop: 16 }}>
        This demo runs entirely in your browser on example data. Nothing is sent anywhere. Sign-in and sync across your devices come with the self-hosted server.
      </div>
      <div className="dim small" style={{ marginTop: 22, lineHeight: 1.6 }}>
        <a href={REPO} target="_blank" rel="noopener">Self-host it in a minute →</a>
      </div>
    </div>
  )

  const signedIn = () => {}

  return (
    <div className="narrow" style={wrap}>
      {head}
      <div style={{ margin: '18px 0 26px', fontSize: 16, color: '#3a3a3c' }}>你的训练、你的重量、你的档案。</div>
      <div style={{ textAlign: 'start', maxWidth: 420, margin: '0 auto', width: '100%' }}>
        {channel === 'phone' ? <>
          <PhoneForm config={config} onSignedIn={signedIn} />
          <div style={{ height: 14 }} />
          <button onClick={() => setChannel('wechat')}
            style={{ width: '100%', padding: '13px 0', borderRadius: 14, cursor: 'pointer',
              background: 'none', border: `1.5px solid ${C.hair}`, color: '#1c1c1e',
              fontSize: 15, fontWeight: 600, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8 }}>
            <Icon name="qr" /> 微信扫码登录
          </button>
        </> : <>
          <WechatForm onSwitchBack={() => {}} />
          <div style={{ height: 14 }} />
          <button onClick={() => setChannel('phone')}
            style={{ width: '100%', padding: '13px 0', borderRadius: 14, cursor: 'pointer',
              background: 'none', border: `1.5px solid ${C.hair}`, color: '#1c1c1e',
              fontSize: 15, fontWeight: 600 }}>用手机号登录</button>
        </>}
      </div>
      {canGuest && <>
        <div style={{ height: 14 }} />
        <Button variant="ghost" className="dim" onClick={() => setGuest(true)}>暂不登录，随便看看</Button>
      </>}
      <div className="dim small" style={{ marginTop: 26, lineHeight: 1.5, color: '#6e6e73' }}>
        每个档案独立保存训练计划、训练记录与身体数据。
      </div>
    </div>
  )
}
