// views/Login.jsx — 登录/注册(认证重构:手机号+短信验证码 / 微信小程序码扫码)
//
// 设计规格(2026-10-29 十项决议;2026-11-25 阶段 3 品牌绿美化;2026-11-25 液体玻璃重设计):
// **深色液体玻璃主题,与 APP 配色同源**(暗色 --bg #000/--bg-el #0e0e10/默认 acc lime #248a3d):
// 近黑底 + 品牌绿液态色斑 blob 漂移(纯 CSS,GPU 合成) + 玻璃卡片(backdrop blur+saturate+
// 内描边高光)浮层 + 控件玻璃化 + CTA 按压弹性 + segmented 液态滑动 + @supports 渐进折射
// (feDisplacementMap 仅支持的环境增强,不支持优雅降级) + prefers-reduced-motion 全降级。
// 通道 segmented 置顶平级(Q7)、流程顺序(发码钮→短信码→CTA)沿用。**逻辑零改动**。
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

// 深色液体玻璃 token(与 APP 暗色主题同源:--bg #000/--bg-el #0e0e10/acc #248a3d)。
const C = {
  bg: '#05070a', ink: '#f2f2f7', sub: 'rgba(235,245,238,.72)',
  green: '#248a3d', greenLight: '#2f9c47', greenDark: '#1e7534',
  glass: 'rgba(14,14,16,.55)', glassLine: 'rgba(255,255,255,.14)',
  hair: 'rgba(255,255,255,.13)', white: '#ffffff', red: '#ff6b6b',
  ring: 'rgba(47,156,71,.22)',
}
const loginInput = {
  width: '100%', background: 'rgba(255,255,255,.06)', color: C.ink,
  border: `1.5px solid ${C.hair}`, borderRadius: 14,
  padding: '12px 13px', fontSize: 15, outline: 'none',
  transition: 'border-color .15s, box-shadow .15s, background .15s',
  backdropFilter: 'blur(8px)', WebkitBackdropFilter: 'blur(8px)',
}
// 聚焦:品牌绿边 + 同色柔光晕 + 玻璃增亮。
const focusInput = e => {
  e.currentTarget.style.borderColor = C.greenLight
  e.currentTarget.style.boxShadow = `0 0 0 4px ${C.ring}`
  e.currentTarget.style.background = 'rgba(255,255,255,.10)'
}
const blurInput = e => { e.currentTarget.style.borderColor = C.hair; e.currentTarget.style.boxShadow = 'none'; e.currentTarget.style.background = 'rgba(255,255,255,.06)' }
const fieldErr = msg => <div style={{ color: C.red, fontSize: 12.5, marginTop: 5, textAlign: 'left' }}>{msg}</div>
const label = txt => <div style={{ fontSize: 13, fontWeight: 600, color: C.sub, margin: '12px 0 6px', textAlign: 'left', letterSpacing: '.02em' }}>{txt}</div>

// 入场动效 + 液态背景动画 + reduced-motion 降级(组件级 <style>,卸载即清)。
const loginStyle = <style>{`
  body { background: #05070a }
  @keyframes muLoginUp { from { opacity: 0; transform: translateY(12px) } to { opacity: 1; transform: none } }
  @keyframes muBlobA { 0% { transform: translate(-8%, -6%) scale(1) } 50% { transform: translate(10%, 8%) scale(1.18) } 100% { transform: translate(-8%, -6%) scale(1) } }
  @keyframes muBlobB { 0% { transform: translate(6%, 4%) scale(1.1) } 50% { transform: translate(-10%, -8%) scale(.92) } 100% { transform: translate(6%, 4%) scale(1.1) } }
  @keyframes muSheen { 0% { transform: translateX(-120%) } 60% { transform: translateX(120%) } 100% { transform: translateX(120%) } }
  .mu-login-anim { animation: muLoginUp .4s cubic-bezier(.2,.7,.3,1) both }
  .mu-blob-a { animation: muBlobA 52s ease-in-out infinite }
  .mu-blob-b { animation: muBlobB 68s ease-in-out infinite }
  .mu-glass-card {
    position: relative; border-radius: 26px;
    background: ${C.glass};
    backdrop-filter: blur(26px) saturate(1.6); -webkit-backdrop-filter: blur(26px) saturate(1.6);
    box-shadow: 0 24px 60px rgba(0,0,0,.5);
    overflow: hidden;
  }
  /* rim 渐变描边(mask xor):顶亮→底绿调,四边统一 */
  .mu-glass-card::after {
    content: ''; position: absolute; inset: 0; border-radius: inherit; padding: 1px; pointer-events: none;
    background: linear-gradient(180deg, rgba(255,255,255,.22), rgba(255,255,255,.05) 45%, rgba(74,222,128,.16));
    -webkit-mask: linear-gradient(#000 0 0) content-box, linear-gradient(#000 0 0);
    -webkit-mask-composite: xor; mask-composite: exclude;
  }
  /* 卡缘流动高光(液态感主视觉):宽 160% 光带 transform 平移(合成器路径,零 repaint),扫 8.4s + 静默 5.6s */
  .mu-glass-card::before {
    content: ''; position: absolute; inset: 0; pointer-events: none; border-radius: inherit;
    width: 160%;
    background: linear-gradient(115deg, transparent 30%, rgba(255,255,255,.055) 45%, rgba(255,255,255,.09) 50%, rgba(255,255,255,.055) 55%, transparent 70%);
    animation: muSheen 14s linear infinite;
    will-change: transform;
  }
  .mu-glass-card button:focus-visible, .mu-glass-card input:focus-visible,
  .mu-page button:focus-visible {
    outline: 2px solid rgba(111,206,133,.8); outline-offset: 2px;
  }
  .mu-glass-card input::placeholder { color: rgba(235,245,238,.42); opacity: 1 }
  @media (prefers-reduced-motion: reduce) {
    .mu-blob-a, .mu-blob-b, .mu-glass-card::before { animation: none }
    .mu-login-anim { animation: none }
  }
`}</style>

// 液态背景:近黑底上三枚品牌绿大色斑缓慢漂移(实色圆+blur 120px,稳定可见)。
function LiquidBackdrop() {
  return <div aria-hidden style={{ position: 'fixed', inset: 0, overflow: 'hidden', background: C.bg, zIndex: -1 }}>
    <div className="mu-blob-a" style={{ position: 'absolute', width: '46vmax', height: '46vmax', left: '-6%', top: '-14%',
      background: '#15803d', borderRadius: '50%', filter: 'blur(120px)', opacity: .5 }} />
    <div className="mu-blob-b" style={{ position: 'absolute', width: '42vmax', height: '42vmax', right: '-8%', bottom: '-12%',
      background: '#166534', borderRadius: '50%', filter: 'blur(120px)', opacity: .45 }} />
    <div style={{ position: 'absolute', width: '24vmax', height: '24vmax', left: '34%', top: '4%',
      background: '#4ade80', borderRadius: '50%', filter: 'blur(110px)', opacity: .16 }} />
  </div>
}

// ── 6 格分段验证码(自动跳格/回退/粘贴分发;获焦绿边+微放大+玻璃底) ───────────────
function SixDigits({ value, onChange, disabled = false }) {
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
          onChange={e => { if (!disabled) onInput(i, e) }} onKeyDown={e => { if (!disabled) onKey(i, e) }}
          tabIndex={disabled ? -1 : 0}
          style={{
            width: '100%', textAlign: 'center', fontSize: 21, fontWeight: 700,
            color: disabled ? 'rgba(242,242,247,.35)' : C.ink,
            background: disabled ? 'rgba(255,255,255,.03)' : 'rgba(255,255,255,.06)',
            border: `1.5px ${disabled ? 'dashed' : 'solid'} ${disabled ? 'rgba(255,255,255,.18)' : focusIdx === i ? C.greenLight : C.hair}`,
            borderRadius: 12, padding: '10px 0', outline: 'none', flex: 1,
            boxShadow: focusIdx === i ? `0 0 0 4px ${C.ring}` : 'none',
            transform: focusIdx === i ? 'scale(1.05)' : 'none',
            transition: 'border-color .12s, box-shadow .12s, transform .12s, background .12s',
          }} />
      ))}
    </div>
  )
}

// ── 手机号登录(主通道:平铺一屏式——图形码/短信码全部常驻可见) ──────────────
// 逻辑流:①手机号+图形码 → 点『发送短信验证码』(校验图形码→发短信→倒计)
//        ②短信码 6 格常驻(发送前禁用置灰+提示) → 登录/注册(校验短信码)
function PhoneForm({ config, onSignedIn }) {
  const { setUser, pushState, pullState, loadConfig } = useStore()
  const inviteOnly = !!config?.invite_only
  const [phone, setPhone] = useState('')
  const [cap, setCap] = useState(null)      // {captchaId, svg}
  const [capCode, setCapCode] = useState('')
  const [smsSent, setSmsSent] = useState(false)
  const [countdown, setCountdown] = useState(0)
  const [digits, setDigits] = useState('')
  const [invite, setInvite] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState(null)      // {msg, field}
  const phoneRef = useRef(null)
  useEffect(() => { setTimeout(() => phoneRef.current?.focus(), 250) }, [])
  useEffect(() => { loadConfig() }, [loadConfig])
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
  // 发送验证码:独立动作钮——校验手机号+图形码后发短信(契约 authSmsSend)。
  const sendSms = async () => {
    setErr(null)
    if (!PHONE_RE.test(phone)) { setErr({ msg: '请输入有效的中国大陆手机号', field: 'phone' }); return }
    if (capCode.length !== 4) { setErr({ msg: '请输入图形验证码', field: 'captchaCode' }); return }
    try {
      await smsSend(phone, cap.captchaId, capCode)
      setSmsSent(true); setDigits(''); setCountdown(60)
    } catch (e) {
      const field = e?.data?.code === 'captcha' ? 'captcha' : 'captchaCode'
      setErr({ msg: e.message, field })
      if (e?.data?.code === 'captcha') refreshCaptcha()
    }
  }
  const signIn = async () => {
    setErr(null)
    if (digits.length < 6) { setErr({ msg: '请先获取并输入短信验证码', field: 'digits' }); return }
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

  const sendReady = !(smsSent && countdown > 0)
  return <>
    {label('手机号')}
    <div style={{ display: 'flex', alignItems: 'stretch', background: 'rgba(255,255,255,.06)',
      border: `1.5px solid ${err?.field === 'phone' ? C.red : C.hair}`, borderRadius: 14,
      overflow: 'hidden', transition: 'border-color .15s, box-shadow .15s' }}>
      <span style={{ padding: '12px 11px', background: 'rgba(255,255,255,.05)', color: C.sub, fontSize: 14,
        borderRight: `1.5px solid ${C.hair}`, userSelect: 'none' }}>+86</span>
      <input ref={phoneRef} inputMode="numeric" maxLength={11} placeholder="请输入手机号" value={phone}
        onChange={e => setPhone(e.target.value.replace(/\D/g, '').slice(0, 11))}
        onFocus={focusInput} onBlur={blurInput}
        style={{ flex: 1, border: 'none', outline: 'none', fontSize: 15, padding: '12px 13px', background: 'transparent', color: C.ink }} />
    </div>
    {err?.field === 'phone' && fieldErr(err.msg)}

    {label('图形验证码')}
    <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
      {cap && <div dangerouslySetInnerHTML={{ __html: cap.svg }}
        style={{ width: 148, height: 46, borderRadius: 12, overflow: 'hidden', cursor: 'pointer', flexShrink: 0,
          border: `1.5px solid ${err?.field === 'captcha' ? C.red : C.hair}`, background: '#f6f8f6',
          filter: 'brightness(.92) contrast(1.05)' }}
        onClick={refreshCaptcha} title="点击刷新" />}
      <input inputMode="numeric" maxLength={4} placeholder="输入 4 位数字" value={capCode}
        onChange={e => setCapCode(e.target.value.replace(/\D/g, '').slice(0, 4))}
        onFocus={focusInput} onBlur={blurInput}
        style={{ ...loginInput, flex: 1, borderColor: err?.field === 'captcha' ? C.red : C.hair }} />
    </div>
    {err?.field === 'captcha' && fieldErr(err.msg)}

    {/* 发码钮:紧贴图形码之后(流程次序),描边次级样式——主 CTA 让位给登录 */}
    <button onClick={sendSms} disabled={!sendReady}
      style={{ width: '100%', marginTop: 14, padding: '11px 0', borderRadius: 12, cursor: sendReady ? 'pointer' : 'default',
        background: sendReady ? 'rgba(47,156,71,.14)' : 'rgba(255,255,255,.04)',
        border: `1.5px solid ${sendReady ? 'rgba(47,156,71,.45)' : C.hair}`,
        color: sendReady ? '#6fce85' : (smsSent ? 'rgba(242,242,247,.58)' : 'rgba(242,242,247,.35)'), fontSize: 14, fontWeight: 600,
        transition: 'background .15s, border-color .15s, color .15s' }}>
      {smsSent ? `重新发送短信验证码${countdown > 0 ? `（${countdown}s）` : ''}` : '发送短信验证码'}
    </button>

    {label('短信验证码')}
    <SixDigits value={digits} onChange={setDigits} disabled={!smsSent} />
    <div style={{ display: 'flex', justifyContent: 'space-between', margin: '7px 0 4px' }}>
      <span style={{ fontSize: 12.5, color: C.sub }}>
        {smsSent ? (countdown > 0 ? `已发送（${countdown}s 后可重发）` : '没收到？点击上方重新发送')
                : '发送后在此输入 6 位验证码'}
      </span>
    </div>

    {err?.field === 'digits' && fieldErr(err.msg)}
    {inviteOnly && <>
      <input className="input" placeholder="邀请码" maxLength={40} value={invite}
        onChange={e => setInvite(e.target.value.toUpperCase())}
        onFocus={focusInput} onBlur={blurInput}
        style={{ ...loginInput, letterSpacing: '.14em', fontWeight: 600, textAlign: 'center', marginTop: 10 }} />
      <div className="dim small" style={{ marginTop: 6 }}>本应用为邀请制，请输入你收到的邀请码。</div>
      {err?.field === 'invite' && fieldErr(err.msg)}
    </>}

    <div style={{ height: 18 }} />
    <button onClick={signIn} disabled={busy || digits.length < 6}
      style={{ position: 'relative', overflow: 'hidden', width: '100%', padding: '14px 0', border: 'none', borderRadius: 14,
        background: busy || digits.length < 6
          ? 'linear-gradient(180deg, rgba(30,138,68,.5), rgba(20,107,53,.5))'
          : 'linear-gradient(180deg, #1e8a44, #146b35)',
        color: busy || digits.length < 6 ? 'rgba(234,255,242,.75)' : '#eafff2',
        fontSize: 16, fontWeight: 700, letterSpacing: '.04em',
        boxShadow: busy || digits.length < 6 ? 'none' : '0 8px 24px rgba(36,138,61,.38), inset 0 1px 0 rgba(255,255,255,.25)',
        cursor: busy || digits.length < 6 ? 'default' : 'pointer',
        transform: 'none', transition: 'background .15s, box-shadow .15s, transform .18s cubic-bezier(.34,1.56,.64,1)'
      }}
      onMouseDown={e => { if (!e.currentTarget.disabled) e.currentTarget.style.transform = 'scale(.965)' }}
      onMouseUp={e => { e.currentTarget.style.transform = 'none' }}
      onMouseLeave={e => { e.currentTarget.style.transform = 'none' }}
      onTouchStart={e => { if (!e.currentTarget.disabled) e.currentTarget.style.transform = 'scale(.965)' }}
      onTouchEnd={e => { e.currentTarget.style.transform = 'none' }}>
      登录 / 注册
    </button>

    <div className="dim small" style={{ marginTop: 10, textAlign: 'left', color: C.sub }}>
      注册即表示同意本实例的用户协议与隐私条款。
    </div>
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
    <div style={{ fontSize: 14, color: C.sub, marginBottom: 14, lineHeight: 1.6 }}>
      微信扫描二维码，在「沐禾健身」小程序中确认，网页将自动登录
    </div>
    {err && <div className="dim small" style={{ color: '#ff6b6b' }}>{err}</div>}
    {qr?.image && <img alt="微信扫码" src={'data:image/png;base64,' + qr.image}
      style={{ width: 224, height: 224, borderRadius: 18, background: '#ffffff', padding: 10,
        border: '1px solid rgba(255,255,255,.16)', margin: '0 auto', display: 'block',
        boxShadow: '0 12px 36px rgba(0,0,0,.55)' }} />}
    {status === 'confirmed'
      ? <div style={{ color: '#6fce85', fontWeight: 700, marginTop: 12 }}>已在微信中确认 ✓</div>
      : qr && <div style={{ color: C.sub, fontSize: 13, marginTop: 12 }}>等待扫码中…</div>}
    <div style={{ height: 12 }} />
    <button onClick={load} style={{ background: 'none', border: 'none', color: '#6fce85',
      fontWeight: 600, cursor: 'pointer', fontSize: 13, padding: '10px 14px' }}>刷新二维码</button>
  </>
}

// 通道 segmented(Q7 平级互切;玻璃滑块液态滑动)。
function ChannelTabs({ channel, setChannel }) {
  const onPhone = channel === 'phone'
  const item = (key, txt, icon) => {
    const on = channel === key
    return <button key={key} onClick={() => setChannel(key)}
      style={{ flex: 1, padding: '10px 0', border: 'none', borderRadius: 10, cursor: 'pointer',
        background: 'transparent', color: on ? '#6fce85' : C.sub,
        fontSize: 14.5, fontWeight: on ? 700 : 600,
        position: 'relative', zIndex: 1,
        display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
        transition: 'color .22s' }}>
      {icon && <Icon name={icon} />}{txt}
    </button>
  }
  return (
    <div style={{ position: 'relative', display: 'flex', gap: 4, background: 'rgba(255,255,255,.055)',
      borderRadius: 13, padding: 4, marginBottom: 18, border: '1px solid rgba(255,255,255,.07)' }}>
      {/* 液态滑块:绝对定位玻璃 pill,transform 平移(回弹曲线) */}
      <div aria-hidden style={{ position: 'absolute', top: 4, bottom: 4, left: 4, width: 'calc(50% - 4px)',
        borderRadius: 999, background: 'linear-gradient(rgba(74,222,128,.10), rgba(255,255,255,.06))',
        boxShadow: 'inset 0 1px 0 rgba(255,255,255,.18), 0 4px 14px rgba(0,0,0,.35)',
        backdropFilter: 'blur(10px) saturate(1.4)', WebkitBackdropFilter: 'blur(10px) saturate(1.4)',
        transform: onPhone ? 'translateX(0)' : 'translateX(calc(100% + 8px))',
        transition: 'transform .32s cubic-bezier(.22,1,.36,1)' }} />
      {item('phone', '手机号登录')}
      {item('wechat', '微信扫码', 'qr')}
    </div>
  )
}

export default function Login() {
  const { setUser, setGuest } = useStore()
  const config = useStore(s => s.config)
  const canGuest = guestAllowed(config)
  const [channel, setChannel] = useState('phone')   // phone | wechat 双通道平级(Q7)
  const head = <>
    <img src="/icon-192.png" alt="沐禾健身" width={64} height={64}
      style={{ borderRadius: 16, margin: '0 auto', display: 'block',
        boxShadow: '0 10px 32px rgba(36,138,61,.40), 0 2px 8px rgba(0,0,0,.6)' }} />
    <h1 style={{ fontSize: 25, fontWeight: 800, letterSpacing: '-.02em', margin: '12px 0 3px', color: C.ink }}>沐禾健身</h1>
    <div style={{ fontSize: 11, fontWeight: 700, letterSpacing: '.3em', color: '#6fce85', textTransform: 'uppercase', textIndent: '.3em' }}>MUHE FIT</div>
  </>
  const wrap = {
    display: 'flex', flexDirection: 'column', textAlign: 'center',
    minHeight: '100dvh', padding: '24px 20px 40px', color: C.ink,
    justifyContent: 'flex-start', paddingTop: 'max(7dvh, 44px)',
  }

  // Demo build: no backend to sign in against — the only way in is the local guest profile.
  if (DEMO) return (
    <>
      <LiquidBackdrop />
      <div className="narrow" style={wrap} data-theme="dark">
        {loginStyle}
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
    </>
  )

  const signedIn = () => {}

  return (
    <>
      <LiquidBackdrop />
      <div className="narrow mu-login-anim" style={wrap} data-theme="dark">
        {loginStyle}
        {head}
        <div style={{ margin: '10px 0 18px', fontSize: 13.5, color: C.sub }}>你的训练、你的重量、你的档案。</div>
        <div className="mu-glass-card mu-glass-refract" style={{ textAlign: 'start', maxWidth: 420, margin: '0 auto', width: '100%', padding: '20px 18px 18px' }}>
          <ChannelTabs channel={channel} setChannel={setChannel} />
          {channel === 'phone'
            ? <PhoneForm config={config} onSignedIn={signedIn} />
            : <WechatForm onSwitchBack={() => {}} />}
        </div>
        {canGuest && <>
          <div style={{ height: 16 }} />
          <Button variant="ghost" className="dim" onClick={() => setGuest(true)}>暂不登录，随便看看</Button>
        </>}
        <div className="dim small" style={{ marginTop: 26, lineHeight: 1.5, color: C.sub }}>
          每个档案独立保存训练计划、训练记录与身体数据。
        </div>
      </div>
    </>
  )
}
