// views/coach-console/CoachShell.jsx — 教练工作台布局壳(fork;coach 角色闸门)。
// 教练≠管理员:独立路由/入口(user.roles 含 coach),手机端可操作(区别于 admin 只读)。
// 样式复用 admin-console.css(同视觉语言,同一控制台体系)。
import { useEffect, useState } from 'react'
import { NavLink, Navigate, Outlet, useNavigate } from 'react-router-dom'
import { useStore } from '../../store/useStore.js'
import { adminT } from '../../lib/admin/i18n.js'
import Icon from '../../components/Icon.jsx'
import '../../admin-console.css'

export default function CoachShell() {
  const nav = useNavigate()
  const user = useStore(s => s.user)
  const isCoach = user?.roles?.includes('coach') || user?.admin

  if (!isCoach) return (
    <div className="adm-console adm-403">
      <h2>{adminT('未授权')}</h2>
      <p>{adminT('此入口需要教练角色')}</p>
    </div>
  )

  return (
    <div className="adm-console adm-shell">
      <aside className="adm-side">
        <div className="brand">沐禾 · {adminT('教练工作台')}</div>
        <NavLink end to="/coach-console" className={({ isActive }) => 'nav-item' + (isActive ? ' on' : '')}>
          <Icon name="dumbbell" />{adminT('我的学员')}
        </NavLink>
        <NavLink to="/coach-console/team" className={({ isActive }) => 'nav-item' + (isActive ? ' on' : '')}>
          <Icon name="barbell" />{adminT('我的团队')}
        </NavLink>
        <div className="spacer" />
        <div className="adm-identity">
          <span className="adm-badge">{adminT('教练')}</span>
        </div>
        <button className="nav-item adm-back" onClick={() => nav('/home')}><Icon name="house" />{adminT('返回应用')}</button>
      </aside>
      <main className="adm-main">
        <Outlet context={{ isCoach }} />
      </main>
    </div>
  )
}
