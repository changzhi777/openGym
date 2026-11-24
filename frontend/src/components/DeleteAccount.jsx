// Settings → Account → "Delete my account" (App Store rule 5.1.1(v): an app you can sign up in
// lets you leave it from inside the app). The server removes the profile and everything in it
// (POST /api/account/delete, the same clean-up as the admin's delete); the store then leaves this
// device where a sign-out does (useStore deleteAccount). Two steps: the sheet offers the backup
// export first (the very one Data & backup has, handed in by Settings) and asks for the
// account's name typed back; that typed name is the proof.
// fork(Go 栈,2026-11-25):上游版本的第二步走 passkey/密码/手机锁证明(ProveOwner/
// PasswordAuth/DeviceConfirm)——Go 栈认证重构后 passkey/密码已退役,该链路整体删除;
// 证明 = 回显账号名(输入正确的名字即本人;会话本身经手机号验证码签发)。
// 服务端另有绑定手机的短信码 proof(smsCode 字段),UI 暂不强制,契约见 openapi accountDelete。
import { useEffect, useRef, useState } from 'react'
import { useStore } from '../store/useStore.js'
import { useUI } from '../store/useUI.js'
import { t } from '../lib/i18n.js'
import { api } from '../lib/api.js'
import { Row, Button } from './ui.jsx'

const errStyle = { color: 'var(--red)', marginTop: 10 }

// The server's answer in the UI language, by its `code` (api/openapi.yaml, /api/account/delete).
export function deleteAccountError(e) {
  switch (e?.data?.code) {
    case 'confirm-name': return t('That is not your account name.')
    case 'sms-code': return t('Wrong or expired code.')
    case 'last-admin': return t('You’re the only admin here, and other profiles still use this server. Make someone else an admin first.')
  }
  return t('Couldn’t delete the account. Nothing was removed.')
}

export function DeleteAccountSheet({ close, exportBackup, exportBackupZip, hasMedia, done }) {
  const user = useStore(s => s.user)
  const [typed, setTyped] = useState('')
  const [err, setErr] = useState(null)
  const [busy, setBusy] = useState(false)
  const ref = useRef(null)
  const name = user?.name || ''
  const match = !!name && typed.trim() === name
  useEffect(() => { setTimeout(() => ref.current?.focus(), 250) }, [])
  const remove = async ev => {
    ev?.preventDefault?.()
    if (busy) return
    setBusy(true); setErr(null)
    try {
      await useStore.getState().deleteAccount(typed.trim(), {})
      close()
      useUI.getState().toast(t('Your account is gone. Thanks for every rep.'))
      done?.()
    } catch (e) { setErr(deleteAccountError(e)) } finally { setBusy(false) }
  }
  return <>
    <h3>{t('Delete your account?')}</h3>
    <div className="muted small" style={{ marginBottom: 14, lineHeight: 1.5 }}>
      {t('Your profile, workouts, body weight, photos and videos are deleted from this server for good. There is no undo, and not even your admin can bring them back.')}
    </div>
    <div className="small" style={{ marginBottom: 8 }}>{t('Want a copy first? Export a backup. You can import it into openGym anytime.')}</div>
    {hasMedia && exportBackupZip
      ? <Button icon="share" onClick={exportBackupZip}>{t('Export with photos & videos (.zip)')}</Button>
      : <Button icon="share" onClick={exportBackup}>{t('Export backup (JSON)')}</Button>}
    <form onSubmit={remove} noValidate style={{ marginTop: 18 }}>
      <label className="small" htmlFor="delete-account-name" style={{ display: 'block', marginBottom: 6 }}>{t('Type {0} to confirm.', name)}</label>
      <input ref={ref} id="delete-account-name" className="input" placeholder={t('Account name')} autoComplete="off"
        autoCapitalize="none" autoCorrect="off" spellCheck={false} value={typed} onChange={e => setTyped(e.target.value)} />
      <div style={{ height: 14 }} />
      <button type="submit" className="btn danger" disabled={!match || busy}>{t('Delete forever')}</button>
    </form>
    {err && <div className="small" role="alert" style={errStyle}>{err}</div>}
    <div style={{ height: 8 }} />
    <Button variant="ghost" className="dim" onClick={close}>{t('Cancel')}</Button>
  </>
}

// The row on the Account page. `done` runs once the account is gone (Settings goes home).
export function DeleteAccountRow(props) {
  const open = () => useUI.getState().openSheet(close => <DeleteAccountSheet {...props} close={close} />)
  return <Row icon="trash" iconTint="var(--red)" title={t('Delete my account')} subtitle={t('Removes your profile and everything in it from this server.')} danger onClick={open} />
}
