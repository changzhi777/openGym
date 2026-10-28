// views/admin/Placeholder.jsx — 占位页:分期待后端/后续迭代(fork)
//
// 设计 §3 的路由全集一次挂齐,未实现页给明确状态而非 404。
// (邀请码/角色已换真页;本组件现服务于营收占位等少数路由。)
import { useOutletContext } from 'react-router-dom'
import { adminT } from '../../lib/admin/i18n.js'

export default function Placeholder({ phase }) {
  const { readonly } = useOutletContext()
  return (
    <div className="adm-console">
      <div className="adm-403">
        <h2>{adminT('暂无数据')}</h2>
        <p>{adminT('此功能等待后端(Go 重构)就绪后开放')}</p>
        {phase && <p style={{ color: 'var(--label-3)', fontSize: 12 }}>{phase}</p>}
        {readonly && <p>{adminT('手机端为只读')}</p>}
      </div>
    </div>
  )
}
