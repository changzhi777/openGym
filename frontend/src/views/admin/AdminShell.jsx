// views/admin/AdminShell.jsx — 管理后台布局壳(fork,P0)
//
// 设计依据 docs/admin-ui-design.md §3/§4:桌面侧栏(运营/系统两组导航 + 身份卡),
// 移动顶栏 + 横向页签;权限不足渲染页面内 403 面板而非重定向(§5.1)。
// 角色现实:后端(P0)只有 user.admin 布尔 —— UI 按设计的三角色建模,
// 角色来源 user.adminRoles(未来 Go 后端),缺省回落 super_admin,结构就绪。
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useStore } from '../../store/useStore.js'
import { adminT } from '../../lib/admin/i18n.js'
import { MOBILE } from '../../lib/mobile.js'
import Icon from '../../components/Icon.jsx'
import '../../admin-console.css'

const NAV = [
  { group: '运营', items: [
    { to: '/admin', end: true, icon: 'chart', label: '概览' },
    { to: '/admin/users', icon: 'arm', label: '用户' },
    { to: '/admin/audit', icon: 'list', label: '审计' },
    { to: '/admin/roles', icon: 'key', label: '角色与权限' },
    { to: '/admin/invites', icon: 'plus', label: '邀请码' },
  ] },
  { group: '系统', items: [
    { to: '/admin/coach', icon: 'sparkles', label: 'AI 教练' },
  ] },
]

const ROLE_LABEL = { super_admin: '超级管理员', ops: '运营', support: '客服(只读)' }

export default function AdminShell() {
  const nav = useNavigate()
  const user = useStore(s => s.user)
  // 后端尚未提供 adminRoles 时,admin 一律视作 super_admin(设计 §5 权限矩阵的临时回落)
  const role = user?.adminRoles?.[0] || (user?.admin ? 'super_admin' : null)
  const readonly = role === 'support'

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
            {g.items.map(it => (
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
    </div>
  )
}
