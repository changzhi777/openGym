// views/admin/StepUpSheet.jsx — 管理敏感操作二次验证(短信码;认证重构后 step-up)。
// AdminShell 经 setStepUpVerifier 注入:apiAdmin 拦到 403 step-up-required 时弹出,
// 成功 resolve(开 5 分钟窗)后原请求自动重放一次。发码复用公开的 /api/auth/sms/send
// (图形码闸),核验走 /api/admin/step-up(须为当前管理员绑定的手机号)。
import { useEffect, useState } from 'react'
import { api, authCaptcha, smsSend } from '../../lib/api.js'
import { adminT } from '../../lib/admin/i18n.js'

export default function StepUpSheet({ close, onDone, onCancel }) {
  const [phone, setPhone] = useState('')
  const [code, setCode] = useState('')
  const [cap, setCap] = useState(null)
  const [capCode, setCapCode] = useState('')
  const [countdown, setCountdown] = useState(0)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState(null)

  useEffect(() => {
    if (countdown <= 0) return
    const t = setTimeout(() => setCountdown(c => c - 1), 1000)
    return () => clearTimeout(t)
  }, [countdown])
  useEffect(() => { authCaptcha().then(setCap).catch(() => setCap(null)) }, [])

  const send = async () => {
    setErr(null)
    if (!/^1\d{10}$/.test(phone)) { setErr(adminT('请输入有效的中国大陆手机号')); return }
    if (capCode.length !== 4) { setErr(adminT('请输入图形验证码')); return }
    try { await smsSend(phone, cap?.captchaId, capCode); setCountdown(60) }
    catch (e) { setErr(e.message); if (e?.data?.code === 'captcha') authCaptcha().then(setCap).catch(() => {}) }
  }
  const go = async () => {
    setBusy(true); setErr(null)
    try {
      await api('/api/admin/step-up', { method: 'POST', body: JSON.stringify({ phone, code: code.trim() }) })
      onDone?.(); close()
    } catch (e) { setErr(e.message); setBusy(false) }
  }
  const cancel = () => { onCancel?.(new Error('已取消二次验证')); close() }

  return (
    <div>
      <h3>{adminT('敏感操作二次验证')}</h3>
      <div className="adm-usub" style={{ margin: '6px 0 10px' }}>
        {adminT('请输入你绑定本管理账号的手机号与收到的短信验证码。')}
      </div>
      <input className="adm-search" style={{ width: '100%' }} inputMode="numeric" maxLength={11}
        placeholder={adminT('绑定的手机号')} value={phone}
        onChange={e => setPhone(e.target.value.replace(/\D/g, '').slice(0, 11))}
        aria-label={adminT('绑定的手机号')} />
      <div style={{ fontSize: 13.5, fontWeight: 600, margin: '10px 0 5px' }}>{adminT('图形验证码')}</div>
      <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
        {cap && <div dangerouslySetInnerHTML={{ __html: cap.svg }}
          style={{ width: 150, height: 44, borderRadius: 8, overflow: 'hidden', cursor: 'pointer', flexShrink: 0 }}
          onClick={() => authCaptcha().then(setCap).catch(() => {})} title={adminT('点击刷新')} />}
        <input className="adm-search" style={{ flex: 1 }} inputMode="numeric" maxLength={4}
          placeholder={adminT('输入 4 位数字')} value={capCode}
          onChange={e => setCapCode(e.target.value.replace(/\D/g, '').slice(0, 4))} />
      </div>
      <div style={{ height: 10 }} />
      <div style={{ display: 'flex', gap: 8 }}>
        <input className="adm-search" style={{ flex: 1, letterSpacing: '.2em', textAlign: 'center' }}
          inputMode="numeric" maxLength={6} placeholder={adminT('短信验证码')} value={code}
          onChange={e => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))} />
        <button className="adm-plain" disabled={countdown > 0 || !/^1\d{10}$/.test(phone)} onClick={send}>
          {countdown > 0 ? `${countdown}s` : adminT('发送验证码')}
        </button>
      </div>
      {err && <div className="adm-usub" style={{ color: 'var(--bad-text)', margin: '8px 0' }}>{err}</div>}
      <div style={{ height: 12 }} />
      <div className="adm-actions">
        <button className="adm-plain" onClick={cancel}>{adminT('取消')}</button>
        <button className="adm-danger" disabled={busy || !code.trim()} onClick={go}>{adminT('确认')}</button>
      </div>
    </div>
  )
}
