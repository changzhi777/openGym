// views/admin/Placeholder.jsx — P0 占位页:分期待后端/后续迭代(fork)
//
// 设计 §3 的路由全集在 P0 一次挂齐,未实现页给明确状态而非 404:
// 「用户/审计/角色」为 P1/P2 迭代,「邀请」暂引导至旧版后台(功能未丢失)。
import { Link, useOutletContext } from 'react-router-dom'
import { adminT } from '../../lib/admin/i18n.js'

export default function Placeholder({ phase, legacy }) {
  const { readonly } = useOutletContext()
  return (
    <div className="adm-console">
      <div className="adm-403">
        <h2>{adminT('暂无数据')}</h2>
        <p>{adminT('此功能等待后端(Go 重构)就绪后开放')}</p>
        {phase && <p style={{ color: 'var(--label-3)', fontSize: 12 }}>{phase}</p>}
        {legacy && <p><Link to="/admin/legacy">{adminT('邀请码')} → {adminT('刷新')}</Link></p>}
        {readonly && <p>{adminT('手机端为只读')}</p>}
      </div>
    </div>
  )
}
