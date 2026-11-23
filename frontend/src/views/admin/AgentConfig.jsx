// views/admin/AgentConfig.jsx — AI-Native 治理页(批次 6,治理台 v1)。
//
// 两项可配:MAX_TURNS(工具循环硬顶,1-8)与审核词表(敏感子串,命中即拦截)。
// PUT /api/admin/coach/agent-config 后下一对话回合热生效(无重启)。
// provider 连接性等只读信息在「AI 教练」页,本页只管运行时治理。
import { useEffect, useState } from 'react'
import { api } from '../../lib/api.js'
import { apiAdmin } from '../../lib/admin/stepUp.js'
import { adminT } from '../../lib/admin/i18n.js'
import { Button, TextField } from '../../components/ui.jsx'

export default function AdminAgentConfig() {
  const [maxTurns, setMaxTurns] = useState('')
  const [words, setWords] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [okLine, setOkLine] = useState('')

  const load = () => {
    setErr('')
    api('/api/admin/coach/agent-config').then(c => {
      setMaxTurns(String(c.maxTurns))
      setWords((c.reviewWords || []).join(', '))
    }).catch(e => setErr(e.message))
  }
  useEffect(load, [])

  const save = async () => {
    setBusy(true)
    setErr('')
    setOkLine('')
    try {
      const wordsList = words.split(/[,，\n]/).map(w => w.trim()).filter(Boolean)
      // PUT 走 apiAdmin(ops+step-up 敏感操作;2026-11-18 审查轮:裸 api() 遇 403
      // 不触发 step-up 重放,治理页保存在 Go 栈曾直接死掉)。
      const r = await apiAdmin('/api/admin/coach/agent-config', {
        method: 'PUT',
        body: JSON.stringify({ maxTurns: Number(maxTurns), reviewWords: wordsList }),
      })
      setMaxTurns(String(r.maxTurns))
      setWords((r.reviewWords || []).join(', '))
      setOkLine(adminT('已保存,下一回合生效'))
    } catch (e) {
      setErr(e.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="narrow">
      <h2>{adminT('AI 治理')}</h2>
      <div className="muted small" style={{ lineHeight: 1.6, marginBottom: 12 }}>
        {adminT('工具循环轮数上限与敏感词审核表。保存后下一对话回合热生效,无需重启。审核为两道串联:词表命中即拦;其后 LLM 复检,复检失败一律不发送。')}
      </div>
      {err && <div className="adm-empty">{adminT('错误')} — {err}</div>}
      {okLine && <div className="adm-empty">{okLine}</div>}
      <div className="adm-card">
        <h3>{adminT('工具循环')}</h3>
        <div className="muted small" style={{ marginBottom: 8 }}>{adminT('单次回复最多几轮工具调用。数值越大,一次回答能查的资料越多,耗时与 token 同比上升。')}</div>
        <TextField value={maxTurns} placeholder="4" onChange={e => setMaxTurns(e.target.value)} />
      </div>
      <div className="adm-card">
        <h3>{adminT('审核词表')}</h3>
        <div className="muted small" style={{ marginBottom: 8 }}>{adminT('逗号或换行分隔。回复包含任一词即拦截(不区分大小写)。')}</div>
        <textarea
          className="tf"
          value={words}
          onChange={e => setWords(e.target.value)}
          rows={4}
          style={{ width: '100%', resize: 'vertical' }}
          placeholder={'词一, 词二\n词三'}
        />
      </div>
      <Button variant="primary" disabled={busy} onClick={save}>{busy ? adminT('保存中…') : adminT('保存')}</Button>
    </div>
  )
}
